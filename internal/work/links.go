package work

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"cairn/internal/model"
	"cairn/internal/store"
)

var (
	ErrLinkExists  = errors.New("work: that link already exists")
	ErrSelfLink    = errors.New("work: an issue cannot be linked to itself")
	ErrParentCycle = errors.New("work: that parent would create a cycle")
	ErrParentSpace = errors.New("work: parent issue must be in the same space")
)

var linkTypes = []string{model.LinkBlocks, model.LinkRelatesTo, model.LinkDuplicates}

// ListLinks returns all links touching an issue, from that issue's perspective.
func (s *Service) ListLinks(ctx context.Context, orgID, issueKey string) ([]model.IssueLinkView, error) {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListIssueLinks(ctx, orgID, issue.ID)
}

// CreateLink links sourceKey → targetKey with the given type.
func (s *Service) CreateLink(ctx context.Context, orgID, actorID, sourceKey, linkType, targetKey string) (*model.IssueLinkView, error) {
	if !slices.Contains(linkTypes, linkType) {
		return nil, fmt.Errorf("%w: invalid link type", ErrValidation)
	}
	source, err := s.GetIssue(ctx, orgID, sourceKey)
	if err != nil {
		return nil, err
	}
	target, err := s.GetIssue(ctx, orgID, targetKey)
	if err != nil {
		return nil, err
	}
	if source.ID == target.ID {
		return nil, ErrSelfLink
	}
	link, err := s.store.CreateIssueLink(ctx, orgID, source.ID, target.ID, linkType, actorID)
	if err != nil {
		if errors.Is(err, store.ErrLinkExists) {
			return nil, ErrLinkExists
		}
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "issue.linked", "issue", source.ID, map[string]any{
		"link_id": link.ID, "type": linkType, "target": target.Key,
	})
	return &model.IssueLinkView{ID: link.ID, Type: linkType, Direction: "outward", Issue: *target}, nil
}

// DeleteLink removes a link by id.
func (s *Service) DeleteLink(ctx context.Context, orgID, actorID, linkID string) error {
	link, err := s.store.GetIssueLink(ctx, orgID, linkID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteIssueLink(ctx, orgID, linkID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "issue.unlinked", "issue", link.SourceIssueID, map[string]any{"link_id": linkID})
	return nil
}

// validateParent checks a proposed parent assignment for an issue: the parent
// must exist in the same space, not be the issue itself, and not be a descendant
// of the issue (which would form a cycle).
func (s *Service) validateParent(ctx context.Context, orgID string, issue *model.Issue, parentID string) error {
	if parentID == issue.ID {
		return ErrParentCycle
	}
	parent, err := s.store.GetIssueByID(ctx, orgID, parentID)
	if err != nil {
		return err
	}
	if parent.SpaceID != issue.SpaceID {
		return ErrParentSpace
	}
	// Setting issue's parent to `parent` is a cycle iff issue is an ancestor of
	// parent (parent's ancestor chain already contains issue).
	ancestors, err := s.store.IssueAncestorIDs(ctx, orgID, parentID)
	if err != nil {
		return err
	}
	if slices.Contains(ancestors, issue.ID) {
		return ErrParentCycle
	}
	return nil
}
