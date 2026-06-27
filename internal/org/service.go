// Package org holds organization business logic: creation, invitations, and
// membership joining. Tenancy is enforced here and in the store layer.
package org

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cairn/internal/model"
	"cairn/internal/store"
)

var (
	// ErrInvalidInvitation is returned for a missing, expired, or used token.
	ErrInvalidInvitation = errors.New("org: invalid or expired invitation")
	// ErrInvitationEmailMismatch is returned when the invite was for a different email.
	ErrInvitationEmailMismatch = errors.New("org: invitation was issued to a different email")
	// ErrPlatformAdmin is returned when a platform admin attempts to belong to an
	// organization (create one, accept an invite, or be invited).
	ErrPlatformAdmin = errors.New("org: platform admins cannot belong to an organization")
)

// Mailer sends invitation emails. Implemented by internal/email.Sender.
type Mailer interface {
	SendInvitation(to, orgName, inviteURL string) error
}

// Service implements organization workflows.
type Service struct {
	store     *store.DB
	mailer    Mailer
	frontend  string
	inviteTTL time.Duration
}

// NewService builds an org Service.
func NewService(db *store.DB, mailer Mailer, frontendURL string, inviteTTL time.Duration) *Service {
	return &Service{store: db, mailer: mailer, frontend: frontendURL, inviteTTL: inviteTTL}
}

// CreateOrganization creates an org (with the caller as owner), generating a
// unique slug from the name.
func (s *Service) CreateOrganization(ctx context.Context, ownerID, name string) (*model.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("org: name is required")
	}
	slug, err := s.uniqueSlug(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.store.CreateOrganization(ctx, name, slug, ownerID)
}

// InviteResult is returned after creating an invitation.
type InviteResult struct {
	Invitation *model.Invitation
	AcceptURL  string
}

// Invite creates an invitation for an email and sends the invite link. spaceID
// is optional; when set, accepting the invite also adds the user to that space.
func (s *Service) Invite(ctx context.Context, org *model.Organization, inviterID, email, role string, spaceID *string) (*InviteResult, error) {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, fmt.Errorf("org: a valid email is required")
	}

	// A platform admin cannot belong to an organization, so they cannot be invited.
	if existing, err := s.store.GetUserByEmail(ctx, email); err == nil && existing.IsPlatformAdmin {
		return nil, ErrPlatformAdmin
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	rawToken, err := randomToken()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(s.inviteTTL)

	inv, err := s.store.CreateInvitation(ctx, org.ID, email, role, hashToken(rawToken), inviterID, spaceID, expiresAt)
	if err != nil {
		return nil, err
	}

	acceptURL := fmt.Sprintf("%s/accept-invite?token=%s", strings.TrimRight(s.frontend, "/"), rawToken)
	if err := s.mailer.SendInvitation(email, org.Name, acceptURL); err != nil {
		// The invite is persisted; surface the send failure but don't roll back.
		return &InviteResult{Invitation: inv, AcceptURL: acceptURL}, fmt.Errorf("org: invite created but email failed: %w", err)
	}
	return &InviteResult{Invitation: inv, AcceptURL: acceptURL}, nil
}

// Accept validates a raw invite token for the authenticated user and joins them
// to the organization.
func (s *Service) Accept(ctx context.Context, user *model.User, rawToken string) (*model.Organization, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, ErrInvalidInvitation
	}
	if user.IsPlatformAdmin {
		return nil, ErrPlatformAdmin
	}

	inv, err := s.store.GetInvitationByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidInvitation
		}
		return nil, err
	}
	if !inv.Pending(time.Now()) {
		return nil, ErrInvalidInvitation
	}
	if !strings.EqualFold(inv.Email, user.Email) {
		return nil, ErrInvitationEmailMismatch
	}

	if err := s.store.CreateMembership(ctx, inv.OrganizationID, user.ID, inv.Role); err != nil {
		if !errors.Is(err, store.ErrAlreadyMember) {
			return nil, err
		}
		// Already a member: treat acceptance as idempotent.
	}
	// A space-targeted invite also grants access to that space.
	if inv.SpaceID != nil {
		if err := s.store.AddSpaceMember(ctx, inv.OrganizationID, *inv.SpaceID, user.ID); err != nil {
			return nil, err
		}
	}
	if err := s.store.MarkInvitationAccepted(ctx, inv.ID); err != nil {
		return nil, err
	}
	return s.store.GetOrganizationByID(ctx, inv.OrganizationID)
}

func (s *Service) uniqueSlug(ctx context.Context, name string) (string, error) {
	base := slugify(name)
	if base == "" {
		base = "org"
	}
	candidate := base
	for range 5 {
		exists, err := s.store.SlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		suffix, err := randomSuffix()
		if err != nil {
			return "", err
		}
		candidate = base + "-" + suffix
	}
	return "", fmt.Errorf("org: could not generate a unique slug")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("org: random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomSuffix() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("org: random suffix: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
