package work

import (
	"context"
	"fmt"
	"strings"

	"cairn/internal/model"
)

// CreateComment posts a comment on an issue (resolved by key).
func (s *Service) CreateComment(ctx context.Context, orgID, actorID, issueKey, body string) (*model.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: comment body is required", ErrValidation)
	}
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	cm, err := s.store.CreateComment(ctx, orgID, issue.ID, actorID, body)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "comment.created", "comment", cm.ID, map[string]any{"issue_id": issue.ID})
	// Commenting subscribes the author to the issue's activity.
	s.autoWatch(ctx, orgID, issue.ID, actorID)
	// Notify @mentioned users and watchers.
	s.notifyComment(ctx, orgID, actorID, issue, body)
	return cm, nil
}

// ListComments returns an issue's comments oldest-first.
func (s *Service) ListComments(ctx context.Context, orgID, issueKey string) ([]model.Comment, error) {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListCommentsByIssue(ctx, orgID, issue.ID)
}

// UpdateComment edits a comment's body. Only the author may edit it.
func (s *Service) UpdateComment(ctx context.Context, orgID, actorID, id, body string) (*model.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: comment body is required", ErrValidation)
	}
	existing, err := s.store.GetCommentByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if existing.AuthorID == nil || *existing.AuthorID != actorID {
		return nil, ErrForbidden
	}
	cm, err := s.store.UpdateComment(ctx, orgID, id, body)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "comment.updated", "comment", cm.ID, nil)
	// Notify only users newly @mentioned by this edit.
	if added := newMentions(existing.Body, body); len(added) > 0 {
		if issue, err := s.store.GetIssueByID(ctx, orgID, existing.IssueID); err == nil {
			s.notifyMentions(ctx, orgID, actorID, issue, body, added)
		}
	}
	return cm, nil
}

// DeleteComment removes a comment (author only). It returns the deleted comment
// so callers can broadcast its issue scope.
func (s *Service) DeleteComment(ctx context.Context, orgID, actorID, id string) (*model.Comment, error) {
	existing, err := s.store.GetCommentByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if existing.AuthorID == nil || *existing.AuthorID != actorID {
		return nil, ErrForbidden
	}
	if err := s.store.DeleteComment(ctx, orgID, id); err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "comment.deleted", "comment", existing.ID, map[string]any{"issue_id": existing.IssueID})
	return existing, nil
}
