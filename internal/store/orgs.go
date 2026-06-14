package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const orgColumns = `id::text, name, slug, created_by::text, created_at, updated_at`

// CreateOrganization creates an organization and its owner membership atomically.
func (db *DB) CreateOrganization(ctx context.Context, name, slug, ownerID string) (*model.Organization, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin create org: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	org := &model.Organization{}
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, created_by)
		VALUES ($1, $2, $3::uuid)
		RETURNING `+orgColumns,
		name, slug, ownerID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("store: insert org: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO memberships (organization_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'owner')`,
		org.ID, ownerID,
	); err != nil {
		return nil, fmt.Errorf("store: insert owner membership: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit create org: %w", err)
	}
	return org, nil
}

// GetOrganizationByID returns an organization. ErrNotFound if absent.
func (db *DB) GetOrganizationByID(ctx context.Context, id string) (*model.Organization, error) {
	org := &model.Organization{}
	err := db.Pool.QueryRow(ctx,
		`SELECT `+orgColumns+` FROM organizations WHERE id = $1::uuid`, id,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get org: %w", err)
	}
	return org, nil
}

// SlugExists reports whether an organization slug is taken.
func (db *DB) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations WHERE slug = $1)`, slug,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: slug exists: %w", err)
	}
	return exists, nil
}

// UpdateOrganization updates mutable organization fields.
func (db *DB) UpdateOrganization(ctx context.Context, id, name string) (*model.Organization, error) {
	org := &model.Organization{}
	err := db.Pool.QueryRow(ctx, `
		UPDATE organizations SET name = $2, updated_at = now()
		WHERE id = $1::uuid
		RETURNING `+orgColumns,
		id, name,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: update org: %w", err)
	}
	return org, nil
}

// ListOrganizationsForUser returns all organizations the user belongs to.
func (db *DB) ListOrganizationsForUser(ctx context.Context, userID string) ([]model.Organization, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+prefixedOrgColumns("o")+`
		FROM organizations o
		JOIN memberships m ON m.organization_id = o.id
		WHERE m.user_id = $1::uuid
		ORDER BY o.created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list orgs: %w", err)
	}
	defer rows.Close()

	var orgs []model.Organization
	for rows.Next() {
		var o model.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan org: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func prefixedOrgColumns(alias string) string {
	return alias + ".id::text, " + alias + ".name, " + alias + ".slug, " +
		alias + ".created_by::text, " + alias + ".created_at, " + alias + ".updated_at"
}
