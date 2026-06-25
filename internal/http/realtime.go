package http

import (
	"net/http"

	"cairn/internal/realtime"
)

// handleSocketIO authenticates the connection from the access-token cookie, then
// hands it to the Socket.IO hub. It must NOT sit behind the JSON auth middleware
// (that writes a JSON 401); auth is performed inline here.
func (s *Server) handleSocketIO(w http.ResponseWriter, r *http.Request) {
	token := accessTokenFromRequest(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, err := s.auth.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.hub.ServeHTTP(w, r, realtime.User{ID: user.ID, Name: user.Name})
}
