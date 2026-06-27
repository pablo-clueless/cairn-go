package store

import (
	"context"
	"fmt"

	"cairn/internal/model"
)

// AddSpaceMember grants a user access to a space. Idempotent.
func (db *DB) AddSpaceMember(ctx context.Context, orgID, spaceID, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO space_members (organization_id, space_id, user_id)
		VALUES ($1::uuid, $2::uuid, $3::uuid)
		ON CONFLICT (space_id, user_id) DO NOTHING`, orgID, spaceID, userID)
	if err != nil {
		return fmt.Errorf("store: add space member: %w", err)
	}
	return nil
}

// RemoveSpaceMember revokes a user's access to a space. ErrNotFound if not a member.
func (db *DB) RemoveSpaceMember(ctx context.Context, orgID, spaceID, userID string) error {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM space_members
		WHERE organization_id = $1::uuid AND space_id = $2::uuid AND user_id = $3::uuid`,
		orgID, spaceID, userID)
	if err != nil {
		return fmt.Errorf("store: remove space member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsSpaceMember reports whether a user belongs to a space.
func (db *DB) IsSpaceMember(ctx context.Context, spaceID, userID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM space_members WHERE space_id = $1::uuid AND user_id = $2::uuid)`,
		spaceID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: is space member: %w", err)
	}
	return exists, nil
}

// ListSpaceMembers returns the users with access to a space, with profile info.
func (db *DB) ListSpaceMembers(ctx context.Context, orgID, spaceID string) ([]model.Member, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT u.id::text, u.email, u.name, m.role, sm.created_at
		FROM space_members sm
		JOIN users u ON u.id = sm.user_id
		LEFT JOIN memberships m ON m.organization_id = sm.organization_id AND m.user_id = sm.user_id
		WHERE sm.organization_id = $1::uuid AND sm.space_id = $2::uuid
		ORDER BY sm.created_at`, orgID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("store: list space members: %w", err)
	}
	defer rows.Close()
	var members []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("store: scan space member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// ListSpacesForUser returns the spaces in an org a user is a member of.
func (db *DB) ListSpacesForUser(ctx context.Context, orgID, userID string) ([]model.Space, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+spaceColumns+`
		FROM spaces s
		JOIN space_members sm ON sm.space_id = s.id
		WHERE s.organization_id = $1::uuid AND sm.user_id = $2::uuid
		ORDER BY s.key`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list spaces for user: %w", err)
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
