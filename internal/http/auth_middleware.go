package http

import (
	"net/http"
	"strings"
)

// authenticate validates the Bearer access token, loads the user, and stores
// it on the request context. Unauthenticated requests are rejected with 401.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
			return
		}

		userID, err := s.auth.ValidateAccessToken(strings.TrimSpace(token))
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
