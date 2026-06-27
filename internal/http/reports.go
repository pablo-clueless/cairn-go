package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
)

//	@Summary	Sprint velocity
//	@Tags		reports
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.VelocityPoint
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/reports/velocity [get]
func (s *Server) handleVelocity(w http.ResponseWriter, r *http.Request) {
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
	pts, err := s.work.Velocity(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, pts)
}

//	@Summary	Sprint burndown
//	@Tags		reports
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Param		sprint		query	string	true	"Sprint ID"
//	@Success	200			{array}	model.BurndownPoint
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/reports/burndown [get]
func (s *Server) handleBurndown(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	sprintID := r.URL.Query().Get("sprint")
	if sprintID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "sprint is required")
		return
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	pts, err := s.work.Burndown(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"), sprintID)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, pts)
}

//	@Summary	Cumulative flow
//	@Tags		reports
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Param		days		query	int		false	"Window in days (default 30)"
//	@Success	200			{array}	model.CFDPoint
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/reports/cfd [get]
func (s *Server) handleCFD(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if _, ok := s.requireSpaceAccess(w, r, scope); !ok {
		return
	}
	pts, err := s.work.CFD(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"), days)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	respond(w, http.StatusOK, pts)
}
