package http

import (
	"net/http"
	"strconv"

	"cairn/internal/authz"
	"cairn/internal/model"
)

//	@Summary	Search issues
//	@Description	Relevance-ranked full-text search across the org's issues (title, description, key).
//	@Tags		search
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID or slug"
//	@Param		q		query	string	true	"Search query"
//	@Param		limit	query	int		false	"Max results (default 20)"
//	@Success	200		{array}	model.Issue
//	@Router		/orgs/{orgID}/search [get]
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	issues, err := s.work.SearchIssues(r.Context(), scope.Org.ID, r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "search failed")
		return
	}
	if issues == nil {
		issues = []model.Issue{}
	}
	respond(w, http.StatusOK, issues)
}
