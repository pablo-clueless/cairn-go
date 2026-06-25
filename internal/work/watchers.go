package work

import (
	"context"
	"log/slog"

	"cairn/internal/model"
)

// ListWatchers returns the users watching an issue.
func (s *Service) ListWatchers(ctx context.Context, orgID, issueKey string) ([]model.Watcher, error) {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListWatchers(ctx, orgID, issue.ID)
}

// WatchIssue subscribes a user to an issue.
func (s *Service) WatchIssue(ctx context.Context, orgID, issueKey, userID string) error {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return err
	}
	return s.store.AddWatcher(ctx, orgID, issue.ID, userID)
}

// UnwatchIssue unsubscribes a user from an issue.
func (s *Service) UnwatchIssue(ctx context.Context, orgID, issueKey, userID string) error {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return err
	}
	return s.store.RemoveWatcher(ctx, orgID, issue.ID, userID)
}

// ListActivity returns an issue's activity feed (audit events touching it).
func (s *Service) ListActivity(ctx context.Context, orgID, issueKey string) ([]model.ActivityEvent, error) {
	issue, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.ListIssueActivity(ctx, orgID, issue.ID)
}

// autoWatch subscribes a user to an issue best-effort (never blocks the caller).
// Used to auto-subscribe people who engage with an issue (create/comment/assign).
func (s *Service) autoWatch(ctx context.Context, orgID, issueID, userID string) {
	if userID == "" {
		return
	}
	if err := s.store.AddWatcher(ctx, orgID, issueID, userID); err != nil {
		slog.Error("auto-watch failed", "issue_id", issueID, "user_id", userID, "error", err)
	}
}
