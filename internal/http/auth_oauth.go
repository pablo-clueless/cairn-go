package http

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const oauthStateCookie = "cairn_oauth_state"

// handleOAuthLogin redirects the user to the SSO provider's consent screen.
//
//	@Summary	Begin SSO login
//	@Description	Redirects to the provider (google or microsoft) authorization endpoint.
//	@Tags		auth
//	@Param		provider	path	string	true	"google or microsoft"
//	@Success	302
//	@Failure	404	{object}	errorEnvelope
//	@Router		/auth/oauth/{provider} [get]
func (s *Server) handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !s.oauth.Enabled(provider) {
		writeError(w, http.StatusNotFound, "provider_unavailable", "this SSO provider is not configured")
		return
	}

	state, err := randomState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not start login")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/api/v1/auth/oauth",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	url, _ := s.oauth.AuthCodeURL(provider, state)
	http.Redirect(w, r, url, http.StatusFound)
}

// handleOAuthCallback completes the SSO flow and redirects back to the frontend.
//
//	@Summary	SSO callback
//	@Tags		auth
//	@Param		provider	path	string	true	"google or microsoft"
//	@Param		code		query	string	false	"authorization code"
//	@Param		state		query	string	false	"CSRF state"
//	@Success	302
//	@Router		/auth/oauth/{provider}/callback [get]
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	redirectFail := s.cfg.FrontendURL + "/auth/callback?status=error"
	redirectOK := s.cfg.FrontendURL + "/auth/callback?status=success"

	if !s.oauth.Enabled(provider) {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}

	// Validate CSRF state against the cookie, then clear it.
	stateCookie, err := r.Cookie(oauthStateCookie)
	s.clearOAuthState(w)
	if err != nil || r.URL.Query().Get("state") == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}

	token, err := s.oauth.Exchange(r.Context(), provider, code)
	if err != nil {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}
	info, err := s.oauth.UserInfo(r.Context(), provider, token)
	if err != nil {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}

	_, pair, err := s.auth.LoginWithOAuth(r.Context(), provider, info, r.UserAgent())
	if err != nil {
		http.Redirect(w, r, redirectFail, http.StatusFound)
		return
	}

	// Set the refresh cookie; the SPA calls /auth/refresh to obtain an access token.
	s.setRefreshCookie(w, pair)
	http.Redirect(w, r, redirectOK, http.StatusFound)
}

func (s *Server) clearOAuthState(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/api/v1/auth/oauth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
