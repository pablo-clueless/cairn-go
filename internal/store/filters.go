package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const filterColumns = `id::text, name, criteria, is_starred, created_at, updated_at`

func scanFilter(row pgx.Row) (*model.SavedFilter, error) {
	f := &model.SavedFilter{}
	err := row.Scan(&f.ID, &f.Name, &f.Criteria, &f.IsStarred, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

// CreateSavedFilter inserts a saved filter owned by a user within an org.
func (db *DB) CreateSavedFilter(ctx context.Context, orgID, userID, name string, criteria json.RawMessage, starred bool) (*model.SavedFilter, error) {
	if len(criteria) == 0 {
		criteria = json.RawMessage(`{}`)
	}
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO saved_filters (organization_id, user_id, name, criteria, is_starred)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5)
		RETURNING id::text`,
		orgID, userID, name, []byte(criteria), starred,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: create saved filter: %w", err)
	}
	return db.GetSavedFilter(ctx, orgID, userID, id)
}

// ListSavedFilters returns a user's filters in an org (starred first, then name).
func (db *DB) ListSavedFilters(ctx context.Context, orgID, userID string) ([]model.SavedFilter, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+filterColumns+` FROM saved_filters
		WHERE organization_id = $1::uuid AND user_id = $2::uuid
		ORDER BY is_starred DESC, name ASC`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list saved filters: %w", err)
	}
	defer rows.Close()
	var out []model.SavedFilter
	for rows.Next() {
		f, err := scanFilter(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan saved filter: %w", err)
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetSavedFilter fetches one of a user's filters. ErrNotFound if absent.
func (db *DB) GetSavedFilter(ctx context.Context, orgID, userID, id string) (*model.SavedFilter, error) {
	f, err := scanFilter(db.Pool.QueryRow(ctx, `
		SELECT `+filterColumns+` FROM saved_filters
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get saved filter: %w", err)
	}
	return f, nil
}

// FilterPatch carries optional saved-filter changes.
type FilterPatch struct {
	Name      *string
	Criteria  json.RawMessage
	IsStarred *bool
}

// UpdateSavedFilter applies a partial update to a user's filter.
func (db *DB) UpdateSavedFilter(ctx context.Context, orgID, userID, id string, p FilterPatch) (*model.SavedFilter, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE saved_filters SET
			name = COALESCE($4, name),
			criteria = COALESCE($5::jsonb, criteria),
			is_starred = COALESCE($6, is_starred),
			updated_at = now()
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id, p.Name, nullableJSON(p.Criteria), p.IsStarred)
	if err != nil {
		return nil, fmt.Errorf("store: update saved filter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetSavedFilter(ctx, orgID, userID, id)
}

// DeleteSavedFilter removes a user's filter. ErrNotFound if absent.
func (db *DB) DeleteSavedFilter(ctx context.Context, orgID, userID, id string) error {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM saved_filters
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id)
	if err != nil {
		return fmt.Errorf("store: delete saved filter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// nullableJSON returns nil for an empty blob so COALESCE leaves the column intact.
func nullableJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
