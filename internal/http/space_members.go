package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/store"
)

type addSpaceMemberRequest struct {
	UserID string `json:"user_id"`
}

//	@Summary	List space members
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.Member
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/members [get]
func (s *Server) handleListSpaceMembers(w http.ResponseWriter, r *http.Request) {
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
	members, err := s.work.ListSpaceMembers(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if members == nil {
		members = []model.Member{}
	}
	respond(w, http.StatusOK, members)
}

//	@Summary	Add a member to a space
//	@Description	Grants an existing org member access to the space. Requires space access + member role.
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string					true	"Organization ID or slug"
//	@Param		spaceKey	path	string					true	"Space key"
//	@Param		body		body	addSpaceMemberRequest	true	"User to add"
//	@Success	204
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/members [post]
func (s *Server) handleAddSpaceMember(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	// Members of a space (and org managers) may bring in other org members.
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	var req addSpaceMemberRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")
		return
	}
	if err := s.work.AddSpaceMember(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), req.UserID); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

//	@Summary	Remove a member from a space
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Param		userID		path	string	true	"User ID to remove"
//	@Success	204
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/members/{userID} [delete]
func (s *Server) handleRemoveSpaceMember(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.RemoveSpaceMember(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), chi.URLParam(r, "userID")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "member not found in this space")
			return
		}
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
