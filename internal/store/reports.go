package store

import (
	"context"
	"fmt"
	"time"

	"cairn/internal/model"
)

// RecordStatusChange appends a status-history row for an issue, resolving the
// status's category. Best-effort callers ignore the error.
func (db *DB) RecordStatusChange(ctx context.Context, orgID, issueID, spaceID, statusID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO issue_status_history (organization_id, issue_id, space_id, status_id, category)
		SELECT $1::uuid, $2::uuid, $3::uuid, st.id, st.category
		FROM workflow_statuses st WHERE st.id = $4::uuid`,
		orgID, issueID, spaceID, statusID)
	if err != nil {
		return fmt.Errorf("store: record status change: %w", err)
	}
	return nil
}

// VelocityBySpace returns completed-sprint velocity points (done vs total issues
// currently assigned to each completed sprint), oldest first.
func (db *DB) VelocityBySpace(ctx context.Context, spaceID string) ([]model.VelocityPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT sp.id::text, sp.name, sp.completed_at,
			count(i.id) FILTER (WHERE st.category = 'done') AS done,
			count(i.id) AS total
		FROM sprints sp
		LEFT JOIN issues i ON i.sprint_id = sp.id
		LEFT JOIN workflow_statuses st ON st.id = i.status_id
		WHERE sp.space_id = $1::uuid AND sp.status = 'completed'
		GROUP BY sp.id, sp.name, sp.completed_at
		ORDER BY sp.completed_at ASC NULLS LAST`, spaceID)
	if err != nil {
		return nil, fmt.Errorf("store: velocity: %w", err)
	}
	defer rows.Close()
	var out []model.VelocityPoint
	for rows.Next() {
		var p model.VelocityPoint
		if err := rows.Scan(&p.SprintID, &p.SprintName, &p.CompletedAt, &p.Completed, &p.Total); err != nil {
			return nil, fmt.Errorf("store: scan velocity: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// StatusChange is one entry in an issue's status timeline.
type StatusChange struct {
	IssueID   string
	Category  string
	ChangedAt time.Time
}

// SprintStatusHistory returns status changes for the issues currently in a sprint,
// ordered oldest-first (for burndown reconstruction).
func (db *DB) SprintStatusHistory(ctx context.Context, sprintID string) ([]StatusChange, error) {
	return db.statusHistory(ctx,
		`SELECT h.issue_id::text, h.category, h.changed_at
		 FROM issue_status_history h
		 JOIN issues i ON i.id = h.issue_id
		 WHERE i.sprint_id = $1::uuid
		 ORDER BY h.changed_at ASC`, sprintID)
}

// SpaceStatusHistory returns status changes for every issue in a space, ordered
// oldest-first (for cumulative-flow reconstruction).
func (db *DB) SpaceStatusHistory(ctx context.Context, spaceID string) ([]StatusChange, error) {
	return db.statusHistory(ctx,
		`SELECT issue_id::text, category, changed_at
		 FROM issue_status_history
		 WHERE space_id = $1::uuid
		 ORDER BY changed_at ASC`, spaceID)
}

func (db *DB) statusHistory(ctx context.Context, query, id string) ([]StatusChange, error) {
	rows, err := db.Pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("store: status history: %w", err)
	}
	defer rows.Close()
	var out []StatusChange
	for rows.Next() {
		var c StatusChange
		if err := rows.Scan(&c.IssueID, &c.Category, &c.ChangedAt); err != nil {
			return nil, fmt.Errorf("store: scan status change: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
