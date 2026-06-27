package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
)

type createLinkRequest struct {
	Type      string `json:"type"`       // blocks | relates_to | duplicates
	TargetKey string `json:"target_key"` // the other issue, e.g. "ENG-12"
}

//	@Summary	List issue links
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	200			{array}	model.IssueLinkView
//	@Router		/orgs/{orgID}/issues/{issueKey}/links [get]
func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	links, err := s.work.ListLinks(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if links == nil {
		links = []model.IssueLinkView{}
	}
	respond(w, http.StatusOK, links)
}

//	@Summary	Link an issue to another
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		issueKey	path		string				true	"Source issue key"
//	@Param		body		body		createLinkRequest	true	"Link"
//	@Success	201			{object}	model.IssueLinkView
//	@Router		/orgs/{orgID}/issues/{issueKey}/links [post]
func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueUpdate) {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	link, err := s.work.CreateLink(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "issueKey"), req.Type, req.TargetKey)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, link)
}

//	@Summary	Remove an issue link
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Param		linkID	path	string	true	"Link ID"
//	@Success	204
//	@Router		/orgs/{orgID}/links/{linkID} [delete]
func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueUpdate) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteLink(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "linkID")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
