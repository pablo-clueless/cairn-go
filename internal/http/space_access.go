package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/model"
)

// isManager reports whether an org role implicitly accesses every space.
func isManager(role string) bool {
	return role == model.RoleOwner || role == model.RoleAdmin
}

// requireSpaceAccessID verifies the caller may access a space (by id). Non-members
// get 404 (not 403) so space existence isn't leaked. Returns false after writing.
func (s *Server) requireSpaceAccessID(w http.ResponseWriter, r *http.Request, scope orgScope, spaceID string) bool {
	user, _ := userFromContext(r.Context())
	ok, err := s.work.CanAccessSpace(r.Context(), spaceID, user.ID, isManager(scope.Role))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not check space access")
		return false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return false
	}
	return true
}

// requireSpaceAccess resolves the {spaceKey} path param, verifies access, and
// returns the space. Returns false (after writing a response) on miss/denial.
func (s *Server) requireSpaceAccess(w http.ResponseWriter, r *http.Request, scope orgScope) (*model.Space, bool) {
	space, err := s.work.GetSpace(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return nil, false
	}
	if !s.requireSpaceAccessID(w, r, scope, space.ID) {
		return nil, false
	}
	return space, true
}

// requireIssueAccess resolves the {issueKey} path param, verifies the caller may
// access its space, and returns the issue. Returns false after writing on miss/denial.
func (s *Server) requireIssueAccess(w http.ResponseWriter, r *http.Request, scope orgScope) (*model.Issue, bool) {
	issue, err := s.work.GetIssue(r.Context(), scope.Org.ID, chi.URLParam(r, "issueKey"))
	if err != nil {
		writeWorkError(w, err)
		return nil, false
	}
	if !s.requireSpaceAccessID(w, r, scope, issue.SpaceID) {
		return nil, false
	}
	return issue, true
}
