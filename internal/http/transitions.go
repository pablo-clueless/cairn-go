package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/work"
)

type transitionItem struct {
	FromStatusID *string `json:"from_status_id"` // null = from any status
	ToStatusID   string  `json:"to_status_id"`
}

type setTransitionsRequest struct {
	Transitions []transitionItem `json:"transitions"`
}

//	@Summary	List workflow transitions
//	@Description	Allowed issue status transitions for a space. An empty list means an open workflow (any status to any other).
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.StatusTransition
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/transitions [get]
func (s *Server) handleListTransitions(w http.ResponseWriter, r *http.Request) {
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
	transitions, err := s.work.ListTransitions(r.Context(), scope.Org.ID, chi.URLParam(r, "spaceKey"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if transitions == nil {
		transitions = []model.StatusTransition{}
	}
	respond(w, http.StatusOK, transitions)
}

//	@Summary	Replace workflow transitions
//	@Description	Replace a space's entire set of allowed status transitions. Send an empty list to reset to an open workflow. A null from_status_id is a global "from any status" transition.
//	@Tags		statuses
//	@Security	BearerAuth
//	@Param		orgID		path	string					true	"Organization ID or slug"
//	@Param		spaceKey	path	string					true	"Space key"
//	@Param		body		body	setTransitionsRequest	true	"Transitions"
//	@Success	200			{array}	model.StatusTransition
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/transitions [put]
func (s *Server) handleSetTransitions(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionStatusManage) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req setTransitionsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}

	in := make([]work.TransitionInput, len(req.Transitions))
	for i, t := range req.Transitions {
		in[i] = work.TransitionInput{From: t.FromStatusID, To: t.ToStatusID}
	}

	transitions, err := s.work.SetTransitions(r.Context(), scope.Org.ID, user.ID, chi.URLParam(r, "spaceKey"), in)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if transitions == nil {
		transitions = []model.StatusTransition{}
	}
	respond(w, http.StatusOK, transitions)
}
