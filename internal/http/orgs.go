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

type createOrgRequest struct {
	Name string `json:"name"`
}

type orgWithRole struct {
	*model.Organization
	Role string `json:"role,omitempty"`
}

// handleCreateOrg creates an organization owned by the caller.
//
//	@Summary	Create organization
//	@Tags		organizations
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createOrgRequest	true	"Org payload"
//	@Success	201		{object}	model.Organization
//	@Failure	400		{object}	errorEnvelope
//	@Router		/orgs [post]
func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())

	var req createOrgRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "organization name is required")
		return
	}

	organization, err := s.orgs.CreateOrganization(r.Context(), user.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create organization")
		return
	}
	respond(w, http.StatusCreated, organization)
}

// handleListOrgs lists the organizations the caller belongs to.
//
//	@Summary	List my organizations
//	@Tags		organizations
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	model.Organization
//	@Router		/orgs [get]
func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgs, err := s.db.ListOrganizationsForUser(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list organizations")
		return
	}
	if orgs == nil {
		orgs = []model.Organization{}
	}
	respond(w, http.StatusOK, orgs)
}

// handleGetOrg returns a single organization the caller belongs to.
//
//	@Summary	Get organization
//	@Tags		organizations
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path		string	true	"Organization ID"
//	@Success	200		{object}	orgWithRole
//	@Failure	404		{object}	errorEnvelope
//	@Router		/orgs/{orgID} [get]
func (s *Server) handleGetOrg(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	respond(w, http.StatusOK, orgWithRole{Organization: scope.Org, Role: scope.Role})
}

// handleUpdateOrg updates organization fields (admin+).
//
//	@Summary	Update organization
//	@Tags		organizations
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path		string			true	"Organization ID"
//	@Param		body	body		createOrgRequest	true	"Fields to update"
//	@Success	200		{object}	model.Organization
//	@Failure	403		{object}	errorEnvelope
//	@Router		/orgs/{orgID} [patch]
func (s *Server) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionOrgUpdate) {
		return
	}

	var req createOrgRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "organization name is required")
		return
	}

	updated, err := s.db.UpdateOrganization(r.Context(), scope.Org.ID, strings.TrimSpace(req.Name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update organization")
		return
	}
	respond(w, http.StatusOK, updated)
}

// handleListMembers lists an organization's members.
//
//	@Summary	List members
//	@Tags		members
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID"
//	@Success	200		{array}	model.Member
//	@Router		/orgs/{orgID}/members [get]
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	members, err := s.db.ListMembers(r.Context(), scope.Org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list members")
		return
	}
	if members == nil {
		members = []model.Member{}
	}
	respond(w, http.StatusOK, members)
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

// handleUpdateMemberRole changes a member's role (admin+).
//
//	@Summary	Update member role
//	@Tags		members
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID"
//	@Param		userID	path	string	true	"User ID"
//	@Param		body	body	updateRoleRequest	true	"New role"
//	@Success	204
//	@Failure	403	{object}	errorEnvelope
//	@Router		/orgs/{orgID}/members/{userID} [patch]
func (s *Server) handleUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionMemberRoleUpdate) {
		return
	}

	targetID := chi.URLParam(r, "userID")
	var req updateRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if !authz.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid role")
		return
	}

	// Prevent demoting the last owner.
	if req.Role != model.RoleOwner {
		if err := s.guardLastOwner(w, r, scope.Org.ID, targetID); err != nil {
			return
		}
	}

	if err := s.db.UpdateMemberRole(r.Context(), scope.Org.ID, targetID, req.Role); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "member not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveMember removes a member (admin+).
//
//	@Summary	Remove member
//	@Tags		members
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID"
//	@Param		userID	path	string	true	"User ID"
//	@Success	204
//	@Failure	403	{object}	errorEnvelope
//	@Router		/orgs/{orgID}/members/{userID} [delete]
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionMemberRemove) {
		return
	}

	targetID := chi.URLParam(r, "userID")
	if err := s.guardLastOwner(w, r, scope.Org.ID, targetID); err != nil {
		return
	}

	if err := s.db.DeleteMembership(r.Context(), scope.Org.ID, targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "member not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not remove member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// guardLastOwner writes an error and returns non-nil if removing/demoting the
// target would leave the org without an owner.
func (s *Server) guardLastOwner(w http.ResponseWriter, r *http.Request, orgID, targetID string) error {
	role, err := s.db.GetMembershipRole(r.Context(), orgID, targetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "member not found")
			return err
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not verify membership")
		return err
	}
	if role != model.RoleOwner {
		return nil
	}
	owners, err := s.db.CountOwners(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not verify owners")
		return err
	}
	if owners <= 1 {
		err := errors.New("last owner")
		writeError(w, http.StatusBadRequest, "last_owner", "an organization must have at least one owner")
		return err
	}
	return nil
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type inviteResponse struct {
	Invitation *model.Invitation `json:"invitation"`
	AcceptURL  string            `json:"accept_url"`
}

// handleCreateInvite invites someone to the organization (admin+).
//
//	@Summary	Invite a member
//	@Tags		invitations
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path		string			true	"Organization ID"
//	@Param		body	body		inviteRequest	true	"Invite payload"
//	@Success	201		{object}	inviteResponse
//	@Failure	403		{object}	errorEnvelope
//	@Failure	409		{object}	errorEnvelope
//	@Router		/orgs/{orgID}/invitations [post]
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionMemberInvite) {
		return
	}
	user, _ := userFromContext(r.Context())

	var req inviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if req.Role == "" {
		req.Role = model.RoleMember
	}
	// Owners cannot be created via invite; only admin/member/guest.
	if req.Role == model.RoleOwner || !authz.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid role")
		return
	}

	result, err := s.orgs.Invite(r.Context(), scope.Org, user.ID, req.Email, req.Role)
	if err != nil {
		if errors.Is(err, store.ErrInvitePending) {
			writeError(w, http.StatusConflict, "invite_pending", "a pending invitation already exists for this email")
			return
		}
		// Invite may be persisted even if email failed; report the partial result.
		if result != nil {
			respond(w, http.StatusCreated, inviteResponse{Invitation: result.Invitation, AcceptURL: result.AcceptURL})
			return
		}
		writeError(w, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	respond(w, http.StatusCreated, inviteResponse{Invitation: result.Invitation, AcceptURL: result.AcceptURL})
}

// handleListInvites lists pending invitations (admin+).
//
//	@Summary	List pending invitations
//	@Tags		invitations
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID"
//	@Success	200		{array}	model.Invitation
//	@Router		/orgs/{orgID}/invitations [get]
func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionMemberInvite) {
		return
	}
	invs, err := s.db.ListInvitations(r.Context(), scope.Org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list invitations")
		return
	}
	if invs == nil {
		invs = []model.Invitation{}
	}
	respond(w, http.StatusOK, invs)
}

// handleDeleteInvite revokes a pending invitation (admin+).
//
//	@Summary	Revoke an invitation
//	@Tags		invitations
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path	string	true	"Organization ID"
//	@Param		inviteID	path	string	true	"Invitation ID"
//	@Success	204
//	@Router		/orgs/{orgID}/invitations/{inviteID} [delete]
func (s *Server) handleDeleteInvite(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	if !s.authorize(w, scope, authz.ActionMemberInvite) {
		return
	}
	inviteID := chi.URLParam(r, "inviteID")
	if err := s.db.DeleteInvitation(r.Context(), scope.Org.ID, inviteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "invitation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not revoke invitation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type acceptInviteRequest struct {
	Token string `json:"token"`
}

// handleAcceptInvite joins the authenticated user to an org via an invite token.
//
//	@Summary	Accept an invitation
//	@Tags		invitations
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		acceptInviteRequest	true	"Invite token"
//	@Success	200		{object}	model.Organization
//	@Failure	400		{object}	errorEnvelope
//	@Failure	403		{object}	errorEnvelope
//	@Router		/invitations/accept [post]
func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())

	var req acceptInviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}

	organization, err := s.orgs.Accept(r.Context(), user, req.Token)
	if err != nil {
		switch {
		case errors.Is(err, org.ErrInvalidInvitation):
			writeError(w, http.StatusBadRequest, "invalid_invitation", "invalid or expired invitation")
		case errors.Is(err, org.ErrInvitationEmailMismatch):
			writeError(w, http.StatusForbidden, "email_mismatch", "this invitation was issued to a different email")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "could not accept invitation")
		}
		return
	}
	respond(w, http.StatusOK, organization)
}
