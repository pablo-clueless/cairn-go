package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/realtime"
	"cairn/internal/store"
	"cairn/internal/work"
)

type createIssueRequest struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	StatusID    *string `json:"status_id"`
	Priority    string  `json:"priority"`
	AssigneeID  *string `json:"assignee_id"`
	DueDate     *string `json:"due_date"`
}

type updateIssueRequest struct {
	Type        *string  `json:"type"`
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	StatusID    *string  `json:"status_id"`
	Priority    *string  `json:"priority"`
	AssigneeID  *string  `json:"assignee_id"`
	SprintID    *string  `json:"sprint_id"`
	ParentID    *string  `json:"parent_id"`
	DueDate     *string  `json:"due_date"`
	Rank        *float64 `json:"rank"`
}

// @Summary	Create issue
// @Tags		issues
// @Security	BearerAuth
// @Param		orgID		path		string				true	"Organization ID or slug"
// @Param		spaceKey	path		string				true	"Space key"
// @Param		body		body		createIssueRequest	true	"Issue"
// @Success	201			{object}	model.Issue
// @Router		/orgs/{orgID}/spaces/{spaceKey}/issues [post]
func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req createIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	issue, err := s.work.CreateIssue(r.Context(), scope.Org.ID, user.ID, work.CreateIssueInput{
		SpaceKey:    chi.URLParam(r, "spaceKey"),
		Type:        req.Type,
		Title:       req.Title,
		Description: req.Description,
		StatusID:    req.StatusID,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
		DueDate:     req.DueDate,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusCreated, issue)
}

// @Summary	List issues
// @Description	Org-wide issue list with optional filters: space (key), assignee ("me" or user id), status.
// @Tags		issues
// @Security	BearerAuth
// @Param		orgID		path	string	true	"Organization ID or slug"
// @Param		space		query	string	false	"Filter by space key"
// @Param		assignee	query	string	false	"Filter by assignee (me or user id)"
// @Param		status		query	string	false	"Filter by status"
// @Success	200			{array}	model.Issue
// @Router		/orgs/{orgID}/issues [get]
func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	user, _ := userFromContext(r.Context())

	filter := store.IssueFilter{
		StatusID: r.URL.Query().Get("status"), // a status id
		Sprint:   r.URL.Query().Get("sprint"), // "backlog" or a sprint id
		ParentID: r.URL.Query().Get("parent"), // a parent issue id (children of)
	}

	if assignee := r.URL.Query().Get("assignee"); assignee != "" {
		if assignee == "me" {
			filter.AssigneeID = user.ID
		} else {
			filter.AssigneeID = assignee
		}
	}
	if spaceKey := r.URL.Query().Get("space"); spaceKey != "" {
		space, err := s.work.GetSpace(r.Context(), scope.Org.ID, spaceKey)
		if err != nil {
			writeWorkError(w, err)
			return
		}
		filter.SpaceID = space.ID
	}

	issues, err := s.work.ListIssues(r.Context(), scope.Org.ID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list issues")
		return
	}
	if issues == nil {
		issues = []model.Issue{}
	}
	respond(w, http.StatusOK, issues)
}

// @Summary	Get issue
// @Tags		issues
// @Security	BearerAuth
// @Param		orgID		path		string	true	"Organization ID or slug"
// @Param		issueKey	path		string	true	"Issue key, e.g. ENG-123"
// @Success	200			{object}	model.Issue
// @Router		/orgs/{orgID}/issues/{issueKey} [get]
func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	issue, err := s.work.GetIssue(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, issue)
}

// @Summary	Update issue
// @Tags		issues
// @Security	BearerAuth
// @Param		orgID		path		string				true	"Organization ID or slug"
// @Param		issueKey	path		string				true	"Issue key, e.g. ENG-123"
// @Param		body		body		updateIssueRequest	true	"Fields to change (send empty assignee_id to unassign)"
// @Success	200			{object}	model.Issue
// @Router		/orgs/{orgID}/issues/{issueKey} [patch]
func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueUpdate) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req updateIssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	issue, err := s.work.UpdateIssue(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "issueKey"), store.IssueUpdate{
		Type:        req.Type,
		Title:       req.Title,
		Description: req.Description,
		StatusID:    req.StatusID,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
		SprintID:    req.SprintID,
		ParentID:    req.ParentID,
		DueDate:     req.DueDate,
		Rank:        req.Rank,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	s.rt.EmitToIssue(issue.ID, realtime.EventIssueUpdated, issue)
	respond(w, http.StatusOK, issue)
}

// @Summary	Delete issue
// @Tags		issues
// @Security	BearerAuth
// @Param		orgID		path	string	true	"Organization ID or slug"
// @Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
// @Success	204
// @Router		/orgs/{orgID}/issues/{issueKey} [delete]
func (s *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueDelete) {
		return
	}
	user, _ := userFromContext(r.Context())
	if err := s.work.DeleteIssue(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "issueKey")); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
