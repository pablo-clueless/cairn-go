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

type dashboardRequest struct {
	Name    *string         `json:"name"`
	Widgets json.RawMessage `json:"widgets"`
}

//	@Summary	List dashboards
//	@Tags		dashboards
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Success	200		{array}	model.Dashboard
//	@Router		/orgs/{orgID}/dashboards [get]
func (s *Server) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())
	items, err := s.db.ListDashboards(r.Context(), scope.Org.ID, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list dashboards")
		return
	}
	if items == nil {
		items = []model.Dashboard{}
	}
	respond(w, http.StatusOK, items)
}

//	@Summary	Create a dashboard
//	@Tags		dashboards
//	@Security	BearerAuth
//	@Param		orgID	path		string				true	"Organization ID or slug"
//	@Param		body	body		dashboardRequest	true	"Dashboard"
//	@Success	201		{object}	model.Dashboard
//	@Router		/orgs/{orgID}/dashboards [post]
func (s *Server) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req dashboardRequest
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
	dash, err := s.db.CreateDashboard(r.Context(), scope.Org.ID, user.ID, name, req.Widgets)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create dashboard")
		return
	}
	respond(w, http.StatusCreated, dash)
}

//	@Summary	Update a dashboard
//	@Tags		dashboards
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		dashboardID	path		string				true	"Dashboard ID"
//	@Param		body		body		dashboardRequest	true	"Fields to change"
//	@Success	200			{object}	model.Dashboard
//	@Router		/orgs/{orgID}/dashboards/{dashboardID} [patch]
func (s *Server) handleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req dashboardRequest
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
	dash, err := s.db.UpdateDashboard(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "dashboardID"), store.DashboardPatch{
		Name:    req.Name,
		Widgets: req.Widgets,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "dashboard not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update dashboard")
		return
	}
	respond(w, http.StatusOK, dash)
}

//	@Summary	Delete a dashboard
//	@Tags		dashboards
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		dashboardID	path	string	true	"Dashboard ID"
//	@Success	204
//	@Router		/orgs/{orgID}/dashboards/{dashboardID} [delete]
func (s *Server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.db.DeleteDashboard(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "dashboardID")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "dashboard not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not delete dashboard")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
