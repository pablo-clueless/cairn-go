package http

import "net/http"

// requirePlatformAdmin gates routes to platform super-admins. Must run after authenticate.
func (s *Server) requirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
			return
		}
		if !user.IsPlatformAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "platform admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
