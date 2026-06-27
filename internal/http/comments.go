package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/realtime"
)

type commentBody struct {
	Body string `json:"body"`
}

//	@Summary	List issue comments
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	200			{array}	model.Comment
//	@Router		/orgs/{orgID}/issues/{issueKey}/comments [get]
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
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
	comments, err := s.work.ListComments(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if comments == nil {
		comments = []model.Comment{}
	}
	respond(w, http.StatusOK, comments)
}

//	@Summary	Add a comment
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		orgID		path		string		true	"Organization ID or slug"
//	@Param		issueKey	path		string		true	"Issue key, e.g. ENG-123"
//	@Param		body		body		commentBody	true	"Comment body"
//	@Success	201			{object}	model.Comment
//	@Router		/orgs/{orgID}/issues/{issueKey}/comments [post]
func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionCommentCreate) {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())

	var req commentBody
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	comment, err := s.work.CreateComment(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "issueKey"), req.Body)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	s.rt.EmitToIssue(comment.IssueID, realtime.EventCommentCreated, comment)
	respond(w, http.StatusCreated, comment)
}

//	@Summary	Edit a comment
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		orgID		path		string		true	"Organization ID or slug"
//	@Param		commentID	path		string		true	"Comment ID"
//	@Param		body		body		commentBody	true	"New body"
//	@Success	200			{object}	model.Comment
//	@Router		/orgs/{orgID}/comments/{commentID} [patch]
func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionCommentUpdate) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req commentBody
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	comment, err := s.work.UpdateComment(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "commentID"), req.Body)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	s.rt.EmitToIssue(comment.IssueID, realtime.EventCommentUpdated, comment)
	respond(w, http.StatusOK, comment)
}

//	@Summary	Delete a comment
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		commentID	path	string	true	"Comment ID"
//	@Success	204
//	@Router		/orgs/{orgID}/comments/{commentID} [delete]
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionCommentDelete) {
		return
	}
	user, _ := userFromContext(r.Context())

	comment, err := s.work.DeleteComment(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "commentID"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	s.rt.EmitToIssue(comment.IssueID, realtime.EventCommentDeleted, map[string]string{
		"id":       comment.ID,
		"issue_id": comment.IssueID,
	})
	w.WriteHeader(http.StatusNoContent)
}
