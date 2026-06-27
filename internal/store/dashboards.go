package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const dashboardColumns = `id::text, name, widgets, created_at, updated_at`

func scanDashboard(row pgx.Row) (*model.Dashboard, error) {
	d := &model.Dashboard{}
	err := row.Scan(&d.ID, &d.Name, &d.Widgets, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// CreateDashboard inserts a dashboard owned by a user within an org.
func (db *DB) CreateDashboard(ctx context.Context, orgID, userID, name string, widgets json.RawMessage) (*model.Dashboard, error) {
	if len(widgets) == 0 {
		widgets = json.RawMessage(`[]`)
	}
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO dashboards (organization_id, user_id, name, widgets)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb)
		RETURNING id::text`,
		orgID, userID, name, []byte(widgets),
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: create dashboard: %w", err)
	}
	return db.GetDashboard(ctx, orgID, userID, id)
}

// ListDashboards returns a user's dashboards in an org (name order).
func (db *DB) ListDashboards(ctx context.Context, orgID, userID string) ([]model.Dashboard, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+dashboardColumns+` FROM dashboards
		WHERE organization_id = $1::uuid AND user_id = $2::uuid
		ORDER BY name ASC`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list dashboards: %w", err)
	}
	defer rows.Close()
	var out []model.Dashboard
	for rows.Next() {
		d, err := scanDashboard(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan dashboard: %w", err)
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetDashboard fetches one of a user's dashboards. ErrNotFound if absent.
func (db *DB) GetDashboard(ctx context.Context, orgID, userID, id string) (*model.Dashboard, error) {
	d, err := scanDashboard(db.Pool.QueryRow(ctx, `
		SELECT `+dashboardColumns+` FROM dashboards
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get dashboard: %w", err)
	}
	return d, nil
}

// DashboardPatch carries optional dashboard changes.
type DashboardPatch struct {
	Name    *string
	Widgets json.RawMessage
}

// UpdateDashboard applies a partial update to a user's dashboard.
func (db *DB) UpdateDashboard(ctx context.Context, orgID, userID, id string, p DashboardPatch) (*model.Dashboard, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE dashboards SET
			name = COALESCE($4, name),
			widgets = COALESCE($5::jsonb, widgets),
			updated_at = now()
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id, p.Name, nullableJSON(p.Widgets))
	if err != nil {
		return nil, fmt.Errorf("store: update dashboard: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetDashboard(ctx, orgID, userID, id)
}

// DeleteDashboard removes a user's dashboard. ErrNotFound if absent.
func (db *DB) DeleteDashboard(ctx context.Context, orgID, userID, id string) error {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM dashboards
		WHERE organization_id = $1::uuid AND user_id = $2::uuid AND id = $3::uuid`,
		orgID, userID, id)
	if err != nil {
		return fmt.Errorf("store: delete dashboard: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
