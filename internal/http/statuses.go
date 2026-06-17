package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
)

type createStatusRequest struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

type updateStatusRequest struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Position *int    `json:"position"`
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
	status, err := s.work.CreateStatus(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), req.Name, req.Category)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, status)
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
	status, err := s.work.UpdateStatus(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "statusID"), req.Name, req.Category, req.Position)
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
