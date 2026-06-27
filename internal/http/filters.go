package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/store"
)

type filterRequest struct {
	Name      *string         `json:"name"`
	Criteria  json.RawMessage `json:"criteria"`
	IsStarred *bool           `json:"is_starred"`
}

//	@Summary	List saved filters
//	@Tags		filters
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Success	200		{array}	model.SavedFilter
//	@Router		/orgs/{orgID}/filters [get]
func (s *Server) handleListFilters(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())
	filters, err := s.db.ListSavedFilters(r.Context(), scope.Org.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list filters")
		return
	}
	if filters == nil {
		filters = []model.SavedFilter{}
	}
	respond(w, http.StatusOK, filters)
}

//	@Summary	Create a saved filter
//	@Tags		filters
//	@Security	BearerAuth
//	@Param		orgID	path		string			true	"Organization ID or slug"
//	@Param		body	body		filterRequest	true	"Filter"
//	@Success	201		{object}	model.SavedFilter
//	@Router		/orgs/{orgID}/filters [post]
func (s *Server) handleCreateFilter(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req filterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}
	starred := req.IsStarred != nil && *req.IsStarred
	filter, err := s.db.CreateSavedFilter(r.Context(), scope.Org.ID, user.ID, name, req.Criteria, starred)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create filter")
		return
	}
	respond(w, http.StatusCreated, filter)
}

//	@Summary	Update a saved filter
//	@Tags		filters
//	@Security	BearerAuth
//	@Param		orgID		path		string			true	"Organization ID or slug"
//	@Param		filterID	path		string			true	"Filter ID"
//	@Param		body		body		filterRequest	true	"Fields to change"
//	@Success	200			{object}	model.SavedFilter
//	@Router		/orgs/{orgID}/filters/{filterID} [patch]
func (s *Server) handleUpdateFilter(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req filterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "validation_error", "name cannot be empty")
			return
		}
		req.Name = &trimmed
	}
	filter, err := s.db.UpdateSavedFilter(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "filterID"), store.FilterPatch{
		Name:      req.Name,
		Criteria:  req.Criteria,
		IsStarred: req.IsStarred,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "filter not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update filter")
		return
	}
	respond(w, http.StatusOK, filter)
}

//	@Summary	Delete a saved filter
//	@Tags		filters
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		filterID	path	string	true	"Filter ID"
//	@Success	204
//	@Router		/orgs/{orgID}/filters/{filterID} [delete]
func (s *Server) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.db.DeleteSavedFilter(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "filterID")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "filter not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not delete filter")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
