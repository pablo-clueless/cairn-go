package http

import (
	"net/http"
	"strings"
)

// authenticate validates the Bearer access token, loads the user, and stores
// it on the request context. Unauthenticated requests are rejected with 401.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := accessTokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		userID, err := s.auth.ValidateAccessToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}

		user, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "user no longer exists")
			return
		}

		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
	})
}

// accessTokenFromRequest reads the access token from the httpOnly cookie, or
// falls back to an Authorization: Bearer header (used by tests and Swagger).
func accessTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(accessCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(token)
	}
	return ""
}
