package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const sprintColumns = `sp.id::text, sp.organization_id::text, sp.space_id::text, sp.name, sp.goal,
	sp.status, sp.start_date, sp.end_date, sp.completed_at,
	(SELECT count(*) FROM issues i WHERE i.sprint_id = sp.id) AS issue_count,
	sp.created_at, sp.updated_at`

func scanSprint(row pgx.Row) (*model.Sprint, error) {
	s := &model.Sprint{}
	err := row.Scan(&s.ID, &s.OrganizationID, &s.SpaceID, &s.Name, &s.Goal, &s.Status,
		&s.StartDate, &s.EndDate, &s.CompletedAt, &s.IssueCount, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// CreateSprint inserts a sprint and returns it.
func (db *DB) CreateSprint(ctx context.Context, orgID, spaceID, name string, goal *string, startDate, endDate *time.Time) (*model.Sprint, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO sprints (organization_id, space_id, name, goal, start_date, end_date)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6) RETURNING id::text`,
		orgID, spaceID, name, goal, startDate, endDate,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: create sprint: %w", err)
	}
	return db.GetSprintByID(ctx, orgID, id)
}

// GetSprintByID fetches a sprint scoped to the org. ErrNotFound if absent.
func (db *DB) GetSprintByID(ctx context.Context, orgID, id string) (*model.Sprint, error) {
	s, err := scanSprint(db.Pool.QueryRow(ctx,
		`SELECT `+sprintColumns+` FROM sprints sp WHERE sp.id = $1::uuid AND sp.organization_id = $2::uuid`,
		id, orgID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get sprint: %w", err)
	}
	return s, nil
}

// ListSprintsBySpace returns a space's sprints, active first then newest.
func (db *DB) ListSprintsBySpace(ctx context.Context, orgID, spaceID string) ([]model.Sprint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+sprintColumns+` FROM sprints sp
		WHERE sp.organization_id = $1::uuid AND sp.space_id = $2::uuid
		ORDER BY (sp.status = 'active') DESC, sp.created_at DESC`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list sprints: %w", err)
	}
	defer rows.Close()

	var sprints []model.Sprint
	for rows.Next() {
		s, err := scanSprint(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan sprint: %w", err)
		}
		sprints = append(sprints, *s)
	}
	return sprints, rows.Err()
}

// CountActiveSprints returns how many active sprints a space has.
func (db *DB) CountActiveSprints(ctx context.Context, spaceID string) (int, error) {
	var n int
	err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM sprints WHERE space_id = $1::uuid AND status = 'active'`, spaceID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count active sprints: %w", err)
	}
	return n, nil
}

// UpdateSprint persists mutable sprint fields and status.
func (db *DB) UpdateSprint(ctx context.Context, s *model.Sprint) (*model.Sprint, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE sprints SET name = $2, goal = $3, status = $4, start_date = $5, end_date = $6,
			completed_at = $7, updated_at = now()
		WHERE id = $1::uuid`,
		s.ID, s.Name, s.Goal, s.Status, s.StartDate, s.EndDate, s.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: update sprint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetSprintByID(ctx, s.OrganizationID, s.ID)
}

// DeleteSprint removes a sprint (its issues fall back to the backlog via FK).
func (db *DB) DeleteSprint(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM sprints WHERE id = $1::uuid AND organization_id = $2::uuid`, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("store: delete sprint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveIncompleteIssuesToBacklog clears the sprint from non-done issues (used on
// sprint completion). Done issues stay in the completed sprint for history.
func (db *DB) MoveIncompleteIssuesToBacklog(ctx context.Context, sprintID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE issues SET sprint_id = NULL, updated_at = now()
		 WHERE sprint_id = $1::uuid
		   AND status_id NOT IN (SELECT id FROM workflow_statuses WHERE category = 'done')`, sprintID,
	)
	if err != nil {
		return fmt.Errorf("store: move incomplete issues: %w", err)
	}
	return nil
}
