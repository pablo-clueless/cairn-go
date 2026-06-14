package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/store"
)

// orgContext resolves the {orgID} path param, verifies the authenticated user
// is a member, and attaches the org + role to the request context. It must run
// after authenticate.
func (s *Server) orgContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := userFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
			return
		}

		orgID := chi.URLParam(r, "orgID")
		org, err := s.db.GetOrganizationByID(r.Context(), orgID)
		if err != nil {
			// Treat unknown/invalid ids as not found to avoid leaking existence.
			writeError(w, http.StatusNotFound, "not_found", "organization not found")
			return
		}

		role, err := s.db.GetMembershipRole(r.Context(), org.ID, user.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Member-only visibility: non-members get 404, not 403.
				writeError(w, http.StatusNotFound, "not_found", "organization not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "could not resolve membership")
			return
		}

		next.ServeHTTP(w, r.WithContext(withOrg(r.Context(), org, role)))
	})
}

// requireOrg fetches the resolved org scope or writes a 500 (programmer error).
func (s *Server) requireOrg(w http.ResponseWriter, r *http.Request) (orgScope, bool) {
	scope, ok := orgFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "missing org context")
		return orgScope{}, false
	}
	return scope, true
}

// authorize checks the caller's role against an action, writing 403 if denied.
func (s *Server) authorize(w http.ResponseWriter, scope orgScope, action authz.Action) bool {
	if !authz.Can(scope.Role, action) {
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to perform this action")
		return false
	}
	return true
}
