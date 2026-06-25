package store

import (
	"context"
	"fmt"

	"cairn/internal/model"
)

// ListIssueActivity returns audit events touching an issue, newest first. It
// matches events whose entity is the issue itself plus events that reference the
// issue via metadata.issue_id (e.g. comments, links).
func (db *DB) ListIssueActivity(ctx context.Context, orgID, issueID string) ([]model.ActivityEvent, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id::text, a.action, a.actor_id::text, u.name, a.entity_type, a.entity_id::text,
			a.metadata, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.organization_id = $1::uuid
		  AND (a.entity_id = $2::uuid OR a.metadata->>'issue_id' = $2)
		ORDER BY a.created_at DESC
		LIMIT 200`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: list issue activity: %w", err)
	}
	defer rows.Close()
	var out []model.ActivityEvent
	for rows.Next() {
		var e model.ActivityEvent
		if err := rows.Scan(&e.ID, &e.Action, &e.ActorID, &e.ActorName, &e.EntityType, &e.EntityID,
			&e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan activity: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
