package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"cairn/internal/auth"
	"cairn/internal/model"
)

const (
	accessCookieName  = "access_token"
	refreshCookieName = "refresh_token"
	// refreshCookiePath scopes the refresh cookie to the auth endpoints only.
	refreshCookiePath = "/v1/auth"
)

type signupRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type authResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        userDTO   `json:"user"`
}

func toUserDTO(u *model.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt}
}

// handleSignup registers a new user and returns an access token.
//
//	@Summary	Register a new user
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		signupRequest	true	"Signup payload"
//	@Success	201		{object}	authResponse
//	@Failure	400		{object}	errorEnvelope
//	@Failure	409		{object}	errorEnvelope
//	@Router		/auth/signup [post]
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if msg, ok := validateSignup(req); !ok {
		writeError(w, http.StatusBadRequest, "validation_error", msg)
		return
	}

	user, pair, err := s.auth.Signup(r.Context(), req.Email, req.Name, req.Password, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email_taken", "an account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create account")
		return
	}

	s.setAuthCookies(w, pair)
	respond(w, http.StatusCreated, authResponse{
		AccessToken: pair.AccessToken,
		ExpiresAt:   pair.AccessExpiresAt,
		User:        toUserDTO(user),
	})
}

// handleLogin authenticates a user and returns an access token.
//
//	@Summary	Log in
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		loginRequest	true	"Login payload"
//	@Success	200		{object}	authResponse
//	@Failure	400		{object}	errorEnvelope
//	@Failure	401		{object}	errorEnvelope
//	@Router		/auth/login [post]
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email and password are required")
		return
	}

	user, pair, err := s.auth.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not log in")
		return
	}

	s.setAuthCookies(w, pair)
	respond(w, http.StatusOK, authResponse{
		AccessToken: pair.AccessToken,
		ExpiresAt:   pair.AccessExpiresAt,
		User:        toUserDTO(user),
	})
}

// handleRefresh rotates the refresh cookie and returns a new access token.
//
//	@Summary	Refresh access token
//	@Description	Uses the httpOnly refresh cookie to mint a new access token (and rotate the refresh token).
//	@Tags		auth
//	@Produce	json
//	@Success	200	{object}	authResponse
//	@Failure	401	{object}	errorEnvelope
//	@Router		/auth/refresh [post]
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing refresh token")
		return
	}

	user, pair, err := s.auth.Refresh(r.Context(), cookie.Value, r.UserAgent())
	if err != nil {
		s.clearAuthCookies(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired refresh token")
		return
	}

	s.setAuthCookies(w, pair)
	respond(w, http.StatusOK, authResponse{
		AccessToken: pair.AccessToken,
		ExpiresAt:   pair.AccessExpiresAt,
		User:        toUserDTO(user),
	})
}

// handleLogout revokes the refresh token and clears the cookie.
//
//	@Summary	Log out
//	@Tags		auth
//	@Produce	json
//	@Success	204	"No Content"
//	@Router		/auth/logout [post]
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		_ = s.auth.Logout(r.Context(), cookie.Value)
	}
	s.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the currently authenticated user.
//
//	@Summary	Current user
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	userDTO
//	@Failure	401	{object}	errorEnvelope
//	@Router		/me [get]
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	respond(w, http.StatusOK, toUserDTO(user))
}

// setAuthCookies sets the httpOnly access (path /) and refresh (path /v1/auth)
// cookies. The frontend reads neither directly; both ride automatically.
func (s *Server) setAuthCookies(w http.ResponseWriter, pair *auth.TokenPair) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessCookieName,
		Value:    pair.AccessToken,
		Path:     "/",
		Expires:  pair.AccessExpiresAt,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    pair.RefreshToken,
		Path:     refreshCookiePath,
		Expires:  pair.RefreshExpiresAt,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookies expires both auth cookies.
func (s *Server) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: accessCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: "", Path: refreshCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func validateSignup(req signupRequest) (string, bool) {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required", false
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		return "a valid email is required", false
	}
	if len(req.Password) < 8 {
		return "password must be at least 8 characters", false
	}
	return "", true
}
