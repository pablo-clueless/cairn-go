package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

const spaceColumns = `s.id::text, s.organization_id::text, s.key, s.name, s.description,
	s.lead_id::text, s.created_by::text, s.created_at, s.updated_at,
	(SELECT count(*) FROM issues i WHERE i.space_id = s.id) AS issue_count`

func scanSpace(row pgx.Row) (*model.Space, error) {
	sp := &model.Space{}
	err := row.Scan(&sp.ID, &sp.OrganizationID, &sp.Key, &sp.Name, &sp.Description,
		&sp.LeadID, &sp.CreatedBy, &sp.CreatedAt, &sp.UpdatedAt, &sp.IssueCount)
	return sp, err
}

// CreateSpace inserts a space and seeds its default workflow statuses
// (To Do / In Progress / Done) atomically. Returns ErrSpaceKeyTaken on a dup key.
func (db *DB) CreateSpace(ctx context.Context, orgID, key, name string, description, leadID *string, createdBy string) (*model.Space, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin create space: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var spaceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO spaces (organization_id, key, name, description, lead_id, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6::uuid) RETURNING id::text`,
		orgID, key, name, description, leadID, createdBy,
	).Scan(&spaceID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrSpaceKeyTaken
		}
		return nil, fmt.Errorf("store: insert space: %w", err)
	}

	defaults := []struct {
		name, category, color string
		position              int
	}{
		{"To Do", "todo", "#6B7280", 0},
		{"In Progress", "in_progress", "#3B82F6", 1},
		{"Done", "done", "#22C55E", 2},
	}
	for _, d := range defaults {
		if _, err := tx.Exec(ctx, `
			INSERT INTO workflow_statuses (organization_id, space_id, name, category, color, position)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`,
			orgID, spaceID, d.name, d.category, d.color, d.position,
		); err != nil {
			return nil, fmt.Errorf("store: seed status: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit create space: %w", err)
	}
	return db.GetSpaceByKey(ctx, orgID, key)
}

// GetSpaceByKey fetches a space by org + key. ErrNotFound if absent.
func (db *DB) GetSpaceByKey(ctx context.Context, orgID, key string) (*model.Space, error) {
	sp, err := scanSpace(db.Pool.QueryRow(ctx,
		`SELECT `+spaceColumns+` FROM spaces s WHERE s.organization_id = $1::uuid AND s.key = $2`,
		orgID, key,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get space: %w", err)
	}
	return sp, nil
}

// ListSpaces returns an organization's spaces.
func (db *DB) ListSpaces(ctx context.Context, orgID string) ([]model.Space, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+spaceColumns+` FROM spaces s WHERE s.organization_id = $1::uuid ORDER BY s.key`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list spaces: %w", err)
	}
	defer rows.Close()

	var spaces []model.Space
	for rows.Next() {
		sp, err := scanSpace(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan space: %w", err)
		}
		spaces = append(spaces, *sp)
	}
	return spaces, rows.Err()
}

// UpdateSpace updates mutable space fields by org + key. ErrNotFound if absent.
func (db *DB) UpdateSpace(ctx context.Context, orgID, key, name string, description, leadID *string) (*model.Space, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE spaces SET name = $3, description = $4, lead_id = $5::uuid, updated_at = now()
		WHERE organization_id = $1::uuid AND key = $2`,
		orgID, key, name, description, leadID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: update space: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetSpaceByKey(ctx, orgID, key)
}

// DeleteSpace removes a space (cascades to its issues). ErrNotFound if absent.
func (db *DB) DeleteSpace(ctx context.Context, orgID, key string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM spaces WHERE organization_id = $1::uuid AND key = $2`, orgID, key,
	)
	if err != nil {
		return fmt.Errorf("store: delete space: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
