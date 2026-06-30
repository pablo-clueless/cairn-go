package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"cairn/internal/authz"
	"cairn/internal/model"
	"cairn/internal/org"
	"cairn/internal/store"
)

type spaceInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// spaceInviteResponse covers both outcomes: an existing org member added straight
// to the space ("added"), or an email invitation sent ("invited").
type spaceInviteResponse struct {
	Status     string            `json:"status"` // "added" | "invited"
	UserID     string            `json:"user_id,omitempty"`
	Invitation *model.Invitation `json:"invitation,omitempty"`
	AcceptURL  string            `json:"accept_url,omitempty"`
}

//	@Summary	Invite someone to a space (by email)
//	@Description	If the email belongs to an existing org member, they're added to the space immediately. Otherwise an org invitation targeting this space is emailed (admin only for brand-new people).
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		spaceKey	path		string				true	"Space key"
//	@Param		body		body		spaceInviteRequest	true	"Invite"
//	@Success	200			{object}	spaceInviteResponse
//	@Success	201			{object}	spaceInviteResponse
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/invitations [post]
func (s *Server) handleInviteToSpace(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	// Space access + member role to invite teammates into the space.
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	space, ok := s.requireSpaceAccess(w, r, scope)
	if !ok {
		return
	}
	inviter, _ := userFromContext(r.Context())

	var req spaceInviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "validation_error", "a valid email is required")
		return
	}
	role := req.Role
	if role == "" {
		role = model.RoleMember
	}
	if role == model.RoleOwner || !authz.ValidRole(role) {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid role")
		return
	}

	// If the email already belongs to an org member, add them to the space directly.
	if existing, err := s.db.GetUserByEmail(r.Context(), email); err == nil {
		if existing.IsPlatformAdmin {
			writeError(w, http.StatusConflict, "platform_admin", "that email belongs to a platform admin and cannot join an organization")
			return
		}
		if _, mErr := s.db.GetMembershipRole(r.Context(), scope.Org.ID, existing.ID); mErr == nil {
			if err := s.work.AddSpaceMember(r.Context(), scope.Org.ID, inviter.ID, space.Key, existing.ID); err != nil {
				writeWorkError(w, err)
				return
			}
			respond(w, http.StatusOK, spaceInviteResponse{Status: "added", UserID: existing.ID})
			return
		}
		// Exists but isn't an org member yet → falls through to an invitation.
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not look up user")
		return
	}

	// Bringing a brand-new person into the org requires admin, like an org invite.
	if !authz.Can(scope.Role, authz.ActionMemberInvite) {
		writeError(w, http.StatusForbidden, "forbidden", "only an admin can invite new people to the organization")
		return
	}

	result, err := s.orgs.Invite(r.Context(), scope.Org, inviter.ID, email, role, &space.ID)
	if err != nil {
		if errors.Is(err, store.ErrInvitePending) {
			writeError(w, http.StatusConflict, "invite_pending", "a pending invitation already exists for this email")
			return
		}
		if errors.Is(err, org.ErrPlatformAdmin) {
			writeError(w, http.StatusConflict, "platform_admin", "that email belongs to a platform admin and cannot join an organization")
			return
		}
		if result != nil {
			respond(w, http.StatusCreated, spaceInviteResponse{Status: "invited", Invitation: result.Invitation, AcceptURL: result.AcceptURL})
			return
		}
		writeError(w, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	respond(w, http.StatusCreated, spaceInviteResponse{Status: "invited", Invitation: result.Invitation, AcceptURL: result.AcceptURL})
}

//	@Summary	List pending space invitations
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Success	200			{array}	model.Invitation
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/invitations [get]
func (s *Server) handleListSpaceInvitations(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionWorkView) {
		return
	}
	space, ok := s.requireSpaceAccess(w, r, scope)
	if !ok {
		return
	}
	invs, err := s.db.ListInvitationsForSpace(r.Context(), scope.Org.ID, space.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list invitations")
		return
	}
	if invs == nil {
		invs = []model.Invitation{}
	}
	respond(w, http.StatusOK, invs)
}

type resendInviteRequest struct {
	Status string `json:"status"` // must be "resent"
}

//	@Summary	Re-send a pending space invitation
//	@Description	Rotates the invite token (invalidating the old link), extends the expiry, and re-sends the email. Body `{"status":"resent"}`.
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path		string				true	"Organization ID or slug"
//	@Param		spaceKey	path		string				true	"Space key"
//	@Param		inviteID	path		string				true	"Invitation ID"
//	@Param		body		body		resendInviteRequest	true	"Set status to resent"
//	@Success	200			{object}	spaceInviteResponse
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/invitations/{inviteID} [patch]
func (s *Server) handleResendSpaceInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	space, ok := s.requireSpaceAccess(w, r, scope)
	if !ok {
		return
	}

	var req resendInviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if req.Status != "resent" {
		writeError(w, http.StatusBadRequest, "validation_error", `status must be "resent"`)
		return
	}

	result, err := s.orgs.ResendSpaceInvitation(r.Context(), scope.Org, space.ID, chi.URLParam(r, "inviteID"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invitation not found")
			return
		}
		// Token was rotated even if the email failed; report the partial result.
		if result != nil {
			respond(w, http.StatusOK, spaceInviteResponse{Status: "invited", Invitation: result.Invitation, AcceptURL: result.AcceptURL})
			return
		}
		writeError(w, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	respond(w, http.StatusOK, spaceInviteResponse{Status: "invited", Invitation: result.Invitation, AcceptURL: result.AcceptURL})
}

//	@Summary	Revoke a pending space invitation
//	@Tags		spaces
//	@Security	BearerAuth
//	@Param		orgID		path	string	true	"Organization ID or slug"
//	@Param		spaceKey	path	string	true	"Space key"
//	@Param		inviteID	path	string	true	"Invitation ID"
//	@Success	204
//	@Router		/orgs/{orgID}/spaces/{spaceKey}/invitations/{inviteID} [delete]
func (s *Server) handleDeleteSpaceInvitation(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionIssueCreate) {
		return
	}
	space, ok := s.requireSpaceAccess(w, r, scope)
	if !ok {
		return
	}
	if err := s.db.DeleteSpaceInvitation(r.Context(), scope.Org.ID, space.ID, chi.URLParam(r, "inviteID")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invitation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not revoke invitation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
