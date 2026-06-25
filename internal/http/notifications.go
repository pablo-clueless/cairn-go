package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/model"
	"cairn/internal/store"
)

//	@Summary	List notifications
//	@Description	The current user's personal inbox (cross-org), newest first.
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		unread	query	bool	false	"Only unread"
//	@Success	200		{array}	model.Notification
//	@Router		/notifications [get]
func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	unreadOnly := r.URL.Query().Get("unread") == "true"
	items, err := s.db.ListNotifications(r.Context(), user.ID, unreadOnly, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list notifications")
		return
	}
	if items == nil {
		items = []model.Notification{}
	}
	respond(w, http.StatusOK, items)
}

//	@Summary	Unread notification count
//	@Tags		notifications
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]int
//	@Router		/notifications/count [get]
func (s *Server) handleNotificationCount(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	n, err := s.db.UnreadCount(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not count notifications")
		return
	}
	respond(w, http.StatusOK, map[string]int{"unread": n})
}

type markReadRequest struct {
	Read bool `json:"read"`
}

//	@Summary	Mark all notifications read
//	@Description	Collection-level update: send {"read": true} to mark every unread notification read.
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		body	body	markReadRequest	true	"Set read=true"
//	@Success	204
//	@Router		/notifications [patch]
func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req markReadRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if !req.Read {
		writeError(w, http.StatusBadRequest, "validation_error", "only marking read is supported")
		return
	}
	if err := s.db.MarkAllNotificationsRead(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

//	@Summary	Mark a notification read
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		id		path	string			true	"Notification ID"
//	@Param		body	body	markReadRequest	true	"Set read=true"
//	@Success	204
//	@Router		/notifications/{id} [patch]
func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req markReadRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if !req.Read {
		writeError(w, http.StatusBadRequest, "validation_error", "only marking read is supported")
		return
	}
	if err := s.db.MarkNotificationRead(r.Context(), user.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "notification not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update notification")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updatePrefsRequest struct {
	EmailMentions    *bool `json:"email_mentions"`
	EmailComments    *bool `json:"email_comments"`
	EmailAssignments *bool `json:"email_assignments"`
}

//	@Summary	Get notification preferences
//	@Tags		notifications
//	@Security	BearerAuth
//	@Success	200	{object}	model.NotificationPreferences
//	@Router		/notifications/preferences [get]
func (s *Server) handleGetNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	prefs, err := s.db.GetNotificationPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load preferences")
		return
	}
	respond(w, http.StatusOK, prefs)
}

//	@Summary	Update notification preferences
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		body	body		updatePrefsRequest	true	"Preferences (omitted fields unchanged)"
//	@Success	200		{object}	model.NotificationPreferences
//	@Router		/notifications/preferences [patch]
func (s *Server) handleUpdateNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req updatePrefsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	// Start from current values so a partial patch leaves the rest intact.
	prefs, err := s.db.GetNotificationPreferences(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load preferences")
		return
	}
	if req.EmailMentions != nil {
		prefs.EmailMentions = *req.EmailMentions
	}
	if req.EmailComments != nil {
		prefs.EmailComments = *req.EmailComments
	}
	if req.EmailAssignments != nil {
		prefs.EmailAssignments = *req.EmailAssignments
	}
	updated, err := s.db.UpsertNotificationPreferences(r.Context(), user.ID, prefs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not save preferences")
		return
	}
	respond(w, http.StatusOK, updated)
}
