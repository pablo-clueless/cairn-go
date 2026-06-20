package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/work"
)

type createStatusRequest struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Color    string `json:"color"`
}

type updateStatusRequest struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Color    *string `json:"color"`
	Position *int    `json:"position"`
}

// bulkUpdateStatusesRequest carries partial changes to several statuses at once
// (e.g. reordering board columns). Each item is matched by id.
type bulkUpdateStatusItem struct {
	ID       string  `json:"id"`
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Color    *string `json:"color"`
	Position *int    `json:"position"`
}

type bulkUpdateStatusesRequest struct {
	Statuses []bulkUpdateStatusItem `json:"statuses"`
}

//	@Summary	List workflow statuses
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.WorkflowStatus
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/statuses [get]
func (s *Server) handleListStatuses(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	statuses, err := s.work.ListStatuses(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if statuses == nil {
		statuses = []model.WorkflowStatus{}
	}
	respond(w, http.StatusOK, statuses)
}

//	@Summary	Create workflow status
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		spaceKey	path		string				true	"Space key"
//	@Param		body		body		createStatusRequest	true	"Status"
//	@Success	201			{object}	model.WorkflowStatus
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/statuses [post]
func (s *Server) handleCreateStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionStatusManage) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	status, err := s.work.CreateStatus(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), req.Name, req.Category, req.Color)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, status)
}

//	@Summary	Bulk update workflow statuses
//	@Description	Apply partial changes to several statuses of a space at once (e.g. reorder columns). Each item is matched by id.
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path	string						true	"Organization ID or slug"
//	@Param		spaceKey	path	string						true	"Space key"
//	@Param		body		body	bulkUpdateStatusesRequest	true	"Statuses to update"
//	@Success	200			{array}	model.WorkflowStatus
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/statuses [patch]
func (s *Server) handleBulkUpdateStatuses(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionStatusManage) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req bulkUpdateStatusesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}

	patches := make([]work.StatusPatchInput, len(req.Statuses))
	for i, it := range req.Statuses {
		patches[i] = work.StatusPatchInput{
			ID:       it.ID,
			Name:     it.Name,
			Category: it.Category,
			Color:    it.Color,
			Position: it.Position,
		}
	}

	statuses, err := s.work.BulkUpdateStatuses(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), patches)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if statuses == nil {
		statuses = []model.WorkflowStatus{}
	}
	respond(w, http.StatusOK, statuses)
}

//	@Summary	Update workflow status
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		statusID	path		string				true	"Status ID"
//	@Param		body		body		updateStatusRequest	true	"Fields"
//	@Success	200			{object}	model.WorkflowStatus
//	@Router		/orgs/{orgID}/statuses/{statusID} [patch]
func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionStatusManage) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req updateStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	status, err := s.work.UpdateStatus(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "statusID"), req.Name, req.Category, req.Color, req.Position)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, status)
}

//	@Summary	Delete workflow status
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		statusID	path	string	true	"Status ID"
//	@Success	204
//	@Router		/orgs/{orgID}/statuses/{statusID} [delete]
func (s *Server) handleDeleteStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionStatusManage) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteStatus(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "statusID")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
