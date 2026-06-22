package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

const statusColumns = `id::text, organization_id::text, space_id::text, name, category, color, position, wip_limit, created_at, updated_at`

func scanStatus(row pgx.Row) (*model.WorkflowStatus, error) {
	s := &model.WorkflowStatus{}
	err := row.Scan(&s.ID, &s.OrganizationID, &s.SpaceID, &s.Name, &s.Category, &s.Color, &s.Position,
		&s.WIPLimit, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// StatusPatch is a partial change to a single status applied in a bulk update.
// Nil fields are left unchanged.
type StatusPatch struct {
	ID       string
	Name     *string
	Category *string
	Color    *string
	Position *int
	WIPLimit *int
}

// ListStatuses returns a space's workflow statuses in board order.
func (db *DB) ListStatuses(ctx context.Context, orgID, spaceID string) ([]model.WorkflowStatus, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+statusColumns+` FROM workflow_statuses
		 WHERE organization_id = $1::uuid AND space_id = $2::uuid ORDER BY position, created_at`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list statuses: %w", err)
	}
	defer rows.Close()

	var statuses []model.WorkflowStatus
	for rows.Next() {
		s, err := scanStatus(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan status: %w", err)
		}
		statuses = append(statuses, *s)
	}
	return statuses, rows.Err()
}

// GetStatus fetches one status scoped to the org. ErrNotFound if absent.
func (db *DB) GetStatus(ctx context.Context, orgID, id string) (*model.WorkflowStatus, error) {
	s, err := scanStatus(db.Pool.QueryRow(ctx,
		`SELECT `+statusColumns+` FROM workflow_statuses WHERE id = $1::uuid AND organization_id = $2::uuid`,
		id, orgID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get status: %w", err)
	}
	return s, nil
}

// CreateStatus appends a status to a space. ErrStatusNameTaken on a dup name.
func (db *DB) CreateStatus(ctx context.Context, orgID, spaceID, name, category, color string, position int) (*model.WorkflowStatus, error) {
	s, err := scanStatus(db.Pool.QueryRow(ctx, `
		INSERT INTO workflow_statuses (organization_id, space_id, name, category, color, position)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6) RETURNING `+statusColumns,
		orgID, spaceID, name, category, color, position,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrStatusNameTaken
		}
		return nil, fmt.Errorf("store: create status: %w", err)
	}
	return s, nil
}

// UpdateStatus updates a status's name, category, color, position, and WIP limit.
func (db *DB) UpdateStatus(ctx context.Context, orgID, id, name, category, color string, position, wipLimit int) (*model.WorkflowStatus, error) {
	s, err := scanStatus(db.Pool.QueryRow(ctx, `
		UPDATE workflow_statuses SET name = $3, category = $4, color = $5, position = $6, wip_limit = $7, updated_at = now()
		WHERE id = $1::uuid AND organization_id = $2::uuid RETURNING `+statusColumns,
		id, orgID, name, category, color, position, wipLimit,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrStatusNameTaken
		}
		return nil, fmt.Errorf("store: update status: %w", err)
	}
	return s, nil
}

// BulkUpdateStatuses applies partial changes to several statuses of one space in
// a single transaction (e.g. reordering columns, which moves multiple at once).
// It returns the space's statuses in board order afterward.
func (db *DB) BulkUpdateStatuses(ctx context.Context, orgID, spaceID string, patches []StatusPatch) ([]model.WorkflowStatus, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin bulk update statuses: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, p := range patches {
		tag, err := tx.Exec(ctx, `
			UPDATE workflow_statuses SET
				name      = COALESCE($4, name),
				category  = COALESCE($5, category),
				color     = COALESCE($6, color),
				position  = COALESCE($7, position),
				wip_limit = COALESCE($8, wip_limit),
				updated_at = now()
			WHERE id = $1::uuid AND organization_id = $2::uuid AND space_id = $3::uuid`,
			p.ID, orgID, spaceID, p.Name, p.Category, p.Color, p.Position, p.WIPLimit,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrStatusNameTaken
			}
			return nil, fmt.Errorf("store: bulk update status %s: %w", p.ID, err)
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrNotFound // an id that isn't a status of this space
		}
	}

	rows, err := tx.Query(ctx,
		`SELECT `+statusColumns+` FROM workflow_statuses
		 WHERE organization_id = $1::uuid AND space_id = $2::uuid ORDER BY position, created_at`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: reload statuses: %w", err)
	}
	defer rows.Close()

	var statuses []model.WorkflowStatus
	for rows.Next() {
		s, err := scanStatus(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan status: %w", err)
		}
		statuses = append(statuses, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit bulk update statuses: %w", err)
	}
	return statuses, nil
}

// DeleteStatus removes a status. ErrStatusInUse if issues still reference it.
func (db *DB) DeleteStatus(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM workflow_statuses WHERE id = $1::uuid AND organization_id = $2::uuid`, id, orgID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrStatusInUse
		}
		return fmt.Errorf("store: delete status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DefaultStatusID returns the first (lowest-position) status of a space.
func (db *DB) DefaultStatusID(ctx context.Context, spaceID string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id::text FROM workflow_statuses WHERE space_id = $1::uuid ORDER BY position, created_at LIMIT 1`,
		spaceID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: default status: %w", err)
	}
	return id, nil
}

// StatusInSpace reports whether a status id belongs to a given space.
func (db *DB) StatusInSpace(ctx context.Context, statusID, spaceID string) (bool, error) {
	var ok bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM workflow_statuses WHERE id = $1::uuid AND space_id = $2::uuid)`,
		statusID, spaceID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: status in space: %w", err)
	}
	return ok, nil
}

// MaxStatusPosition returns the highest position in a space (for appending).
func (db *DB) MaxStatusPosition(ctx context.Context, spaceID string) (int, error) {
	var pos int
	err := db.Pool.QueryRow(ctx,
		`SELECT coalesce(max(position), -1) FROM workflow_statuses WHERE space_id = $1::uuid`, spaceID,
	).Scan(&pos)
	if err != nil {
		return 0, fmt.Errorf("store: max status position: %w", err)
	}
	return pos, nil
}
