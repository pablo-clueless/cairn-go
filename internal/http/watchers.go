package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
)

//	@Summary	List issue watchers
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	200			{array}	model.Watcher
//	@Router		/orgs/{orgID}/issues/{issueKey}/watchers [get]
func (s *Server) handleListWatchers(w http.ResponseWriter, r *http.Request) {
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
	watchers, err := s.work.ListWatchers(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if watchers == nil {
		watchers = []model.Watcher{}
	}
	respond(w, http.StatusOK, watchers)
}

//	@Summary	Watch an issue (subscribe current user)
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	204
//	@Router		/orgs/{orgID}/issues/{issueKey}/watchers [post]
func (s *Server) handleWatchIssue(w http.ResponseWriter, r *http.Request) {
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
	if err := s.work.WatchIssue(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"), user.ID); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

//	@Summary	Unwatch an issue
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Param		userID		path	string	true	"User ID (must be the current user, or an admin)"
//	@Success	204
//	@Router		/orgs/{orgID}/issues/{issueKey}/watchers/{userID} [delete]
func (s *Server) handleUnwatchIssue(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if _, ok := s.requireIssueAccess(w, r, scope); !ok {
		return
	}
	user, _ := userFromContext(r.Context())
	target := chi.URLParam(r, "userID")
	// Users manage their own watch; admins may remove anyone's.
	if target != user.ID && !authz.Can(scope.Role, authz.ActionMemberRemove) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only manage your own watch")
		return
	}
	if err := s.work.UnwatchIssue(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"), target); err != nil {
		writeWorkError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

//	@Summary	Issue activity feed
//	@Tags		issues
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		issueKey	path	string	true	"Issue key, e.g. ENG-123"
//	@Success	200			{array}	model.ActivityEvent
//	@Router		/orgs/{orgID}/issues/{issueKey}/activity [get]
func (s *Server) handleIssueActivity(w http.ResponseWriter, r *http.Request) {
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
	events, err := s.work.ListActivity(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if events == nil {
		events = []model.ActivityEvent{}
	}
	respond(w, http.StatusOK, events)
}
