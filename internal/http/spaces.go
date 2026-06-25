package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/store"
	"cairn/internal/work"
)

type createSpaceRequest struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	LeadID      *string `json:"lead_id"`
}

type updateSpaceRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	LeadID      *string `json:"lead_id"`
}

// writeWorkError maps work-layer errors to HTTP responses.
func writeWorkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, work.ErrSpaceKeyTaken):
		writeError(w, http.StatusConflict, "key_taken", "that space key is already in use")
	case errors.Is(err, work.ErrInvalidKey):
		writeError(w, http.StatusBadRequest, "validation_error", work.ErrInvalidKey.Error())
	case errors.Is(err, work.ErrInvalidIssue):
		writeError(w, http.StatusBadRequest, "validation_error", "invalid issue reference")
	case errors.Is(err, store.ErrStatusNameTaken):
		writeError(w, http.StatusConflict, "status_name_taken", "a status with that name already exists")
	case errors.Is(err, store.ErrStatusInUse):
		writeError(w, http.StatusConflict, "status_in_use", "move its issues to another status before deleting")
	case errors.Is(err, work.ErrActiveSprintExists):
		writeError(w, http.StatusConflict, "active_sprint_exists", work.ErrActiveSprintExists.Error())
	case errors.Is(err, work.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", "invalid sprint status transition")
	case errors.Is(err, work.ErrInvalidIssueTransition):
		writeError(w, http.StatusConflict, "invalid_transition", "that status change isn't allowed by the workflow")
	case errors.Is(err, work.ErrLinkExists):
		writeError(w, http.StatusConflict, "link_exists", work.ErrLinkExists.Error())
	case errors.Is(err, work.ErrSelfLink):
		writeError(w, http.StatusBadRequest, "validation_error", work.ErrSelfLink.Error())
	case errors.Is(err, work.ErrParentCycle):
		writeError(w, http.StatusConflict, "parent_cycle", work.ErrParentCycle.Error())
	case errors.Is(err, work.ErrParentSpace):
		writeError(w, http.StatusBadRequest, "validation_error", work.ErrParentSpace.Error())
	case errors.Is(err, work.ErrValidation):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, work.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you can only modify your own content")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
	}
}

//	@Summary	Create space
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID	path		string				true	"Organization ID or slug"
//	@Param		body	body		createSpaceRequest	true	"Space"
//	@Success	201		{object}	model.Space
//	@Router		/orgs/{orgID}/spaces [post]
func (s *Server) handleCreateSpace(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSpaceCreate) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createSpaceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	space, err := s.work.CreateSpace(r.Context(), scope.Org.ID, user.ID, req.Key, req.Name, req.Description, req.LeadID)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, space)
}

//	@Summary	List spaces
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Success	200		{array}	model.Space
//	@Router		/orgs/{orgID}/spaces [get]
func (s *Server) handleListSpaces(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	spaces, err := s.work.ListSpaces(r.Context(), scope.Org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list spaces")
		return
	}
	if spaces == nil {
		spaces = []model.Space{}
	}
	respond(w, http.StatusOK, spaces)
}

//	@Summary	Get space
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path		string	true	"Organization ID or slug"
//	@Param		spaceKey	path		string	true	"Space key"
//	@Success	200			{object}	model.Space
//	@Router		/orgs/{orgID}/spaces/{spaceKey} [get]
func (s *Server) handleGetSpace(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	space, err := s.work.GetSpace(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, space)
}

//	@Summary	Update space
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		spaceKey	path		string				true	"Space key"
//	@Param		body		body		updateSpaceRequest	true	"Fields"
//	@Success	200			{object}	model.Space
//	@Router		/orgs/{orgID}/spaces/{spaceKey} [patch]
func (s *Server) handleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSpaceUpdate) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req updateSpaceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	space, err := s.work.UpdateSpace(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), req.Name, req.Description, req.LeadID)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, space)
}

//	@Summary	Delete space
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	204
//	@Router		/orgs/{orgID}/spaces/{spaceKey} [delete]
func (s *Server) handleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionSpaceDelete) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteSpace(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
