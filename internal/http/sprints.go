package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/work"
)

type createSprintRequest struct {
	Name      string     `json:"name"`
	Goal      *string    `json:"goal"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
}

type updateSprintRequest struct {
	Name      *string    `json:"name"`
	Goal      *string    `json:"goal"`
	Status    *string    `json:"status"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
}

//	@Summary	List sprints
//	@Tags		sprints
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.Sprint
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/sprints [get]
func (s *Server) handleListSprints(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	sprints, err := s.work.ListSprints(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if sprints == nil {
		sprints = []model.Sprint{}
	}
	respond(w, http.StatusOK, sprints)
}

//	@Summary	Create sprint
//	@Tags		sprints
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		spaceKey	path		string				true	"Space key"
//	@Param		body		body		createSprintRequest	true	"Sprint"
//	@Success	201			{object}	model.Sprint
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/sprints [post]
func (s *Server) handleCreateSprint(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSprintManage) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createSprintRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	sprint, err := s.work.CreateSprint(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), req.Name, req.Goal, req.StartDate, req.EndDate)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, sprint)
}

//	@Summary	Get sprint
//	@Tags		sprints
//	@Security	BearerAuth
//	@Param		orgID		path		string	true	"Organization ID or slug"
//	@Param		sprintID	path		string	true	"Sprint ID"
//	@Success	200			{object}	model.Sprint
//	@Router		/orgs/{orgID}/sprints/{sprintID} [get]
func (s *Server) handleGetSprint(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	sprint, err := s.work.GetSprint(r.Context(), scope.Org.ID, chi.URLParam(r, "sprintID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, sprint.SpaceID) {
		return
	}
	respond(w, http.StatusOK, sprint)
}

//	@Summary	Update sprint (edit, start, complete)
//	@Tags		sprints
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		sprintID	path		string				true	"Sprint ID"
//	@Param		body		body		updateSprintRequest	true	"Fields / status transition"
//	@Success	200			{object}	model.Sprint
//	@Router		/orgs/{orgID}/sprints/{sprintID} [patch]
func (s *Server) handleUpdateSprint(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSprintManage) {
		return
	}
	existing, err := s.work.GetSprint(r.Context(), scope.Org.ID, chi.URLParam(r, "sprintID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, existing.SpaceID) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req updateSprintRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	sprint, err := s.work.UpdateSprint(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "sprintID"), work.SprintUpdate{
		Name:      req.Name,
		Goal:      req.Goal,
		Status:    req.Status,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, sprint)
}

//	@Summary	Delete sprint
//	@Tags		sprints
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		sprintID	path	string	true	"Sprint ID"
//	@Success	204
//	@Router		/orgs/{orgID}/sprints/{sprintID} [delete]
func (s *Server) handleDeleteSprint(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSprintManage) {
		return
	}
	existing, err := s.work.GetSprint(r.Context(), scope.Org.ID, chi.URLParam(r, "sprintID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if !s.requireSpaceAccessID(w, r, scope, existing.SpaceID) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteSprint(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "sprintID")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
