package store

import (
	"context"
	"fmt"

	"cairn/internal/model"
)

// AddWatcher subscribes a user to an issue. Idempotent (no error if already watching).
func (db *DB) AddWatcher(ctx context.Context, orgID, issueID, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO issue_watchers (organization_id, issue_id, user_id)
		VALUES ($1::uuid, $2::uuid, $3::uuid)
		ON CONFLICT (issue_id, user_id) DO NOTHING`, orgID, issueID, userID)
	if err != nil {
		return fmt.Errorf("store: add watcher: %w", err)
	}
	return nil
}

// RemoveWatcher unsubscribes a user from an issue. ErrNotFound if not watching.
func (db *DB) RemoveWatcher(ctx context.Context, orgID, issueID, userID string) error {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM issue_watchers
		WHERE organization_id = $1::uuid AND issue_id = $2::uuid AND user_id = $3::uuid`,
		orgID, issueID, userID)
	if err != nil {
		return fmt.Errorf("store: remove watcher: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListWatchers returns the users watching an issue, with their names/emails.
func (db *DB) ListWatchers(ctx context.Context, orgID, issueID string) ([]model.Watcher, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT w.user_id::text, u.name, u.email, w.created_at
		FROM issue_watchers w JOIN users u ON u.id = w.user_id
		WHERE w.organization_id = $1::uuid AND w.issue_id = $2::uuid
		ORDER BY w.created_at ASC`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: list watchers: %w", err)
	}
	defer rows.Close()
	var out []model.Watcher
	for rows.Next() {
		var wch model.Watcher
		if err := rows.Scan(&wch.UserID, &wch.Name, &wch.Email, &wch.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan watcher: %w", err)
		}
		out = append(out, wch)
	}
	return out, rows.Err()
}

// WatcherIDs returns just the user ids watching an issue (for fan-out).
func (db *DB) WatcherIDs(ctx context.Context, orgID, issueID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id::text FROM issue_watchers
		WHERE organization_id = $1::uuid AND issue_id = $2::uuid`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: watcher ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
