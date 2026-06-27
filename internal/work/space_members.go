package work

import (
	"context"
	"errors"
	"fmt"

	"cairn/internal/model"
	"cairn/internal/store"
)

// CanAccessSpace reports whether a user may access a space: org managers
// (owners/admins) always can; everyone else must be a space member.
func (s *Service) CanAccessSpace(ctx context.Context, spaceID, userID string, isManager bool) (bool, error) {
	if isManager {
		return true, nil
	}
	return s.store.IsSpaceMember(ctx, spaceID, userID)
}

// AccessibleSpaceIDs returns the ids of spaces a user may access (for filtering
// org-wide issue lists). Returns a non-nil (possibly empty) slice.
func (s *Service) AccessibleSpaceIDs(ctx context.Context, orgID, userID string) ([]string, error) {
	spaces, err := s.store.ListSpacesForUser(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(spaces))
	for _, sp := range spaces {
		ids = append(ids, sp.ID)
	}
	return ids, nil
}

// ListSpaceMembers returns the members of a space (resolved by key).
func (s *Service) ListSpaceMembers(ctx context.Context, orgID, spaceKey string) ([]model.Member, error) {
	sp, err := s.GetSpace(ctx, orgID, spaceKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListSpaceMembers(ctx, orgID, sp.ID)
}

// AddSpaceMember grants an existing org member access to a space.
func (s *Service) AddSpaceMember(ctx context.Context, orgID, actorID, spaceKey, userID string) error {
	sp, err := s.GetSpace(ctx, orgID, spaceKey)
	if err != nil {
		return err
	}
	// Only org members can be added to a space.
	if _, err := s.store.GetMembershipRole(ctx, orgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: user is not a member of this organization", ErrValidation)
		}
		return err
	}
	if err := s.store.AddSpaceMember(ctx, orgID, sp.ID, userID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "space.member_added", "space", sp.ID, map[string]any{"user_id": userID})
	return nil
}

// RemoveSpaceMember revokes a user's access to a space.
func (s *Service) RemoveSpaceMember(ctx context.Context, orgID, actorID, spaceKey, userID string) error {
	sp, err := s.GetSpace(ctx, orgID, spaceKey)
	if err != nil {
		return err
	}
	if err := s.store.RemoveSpaceMember(ctx, orgID, sp.ID, userID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "space.member_removed", "space", sp.ID, map[string]any{"user_id": userID})
	return nil
}
