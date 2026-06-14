package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

// GetMembershipRole returns the user's role in an organization. ErrNotFound if not a member.
func (db *DB) GetMembershipRole(ctx context.Context, orgID, userID string) (string, error) {
	var role string
	err := db.Pool.QueryRow(ctx,
		`SELECT role FROM memberships WHERE organization_id = $1::uuid AND user_id = $2::uuid`,
		orgID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: get membership role: %w", err)
	}
	return role, nil
}

// CreateMembership adds a user to an organization. Returns ErrAlreadyMember on duplicate.
func (db *DB) CreateMembership(ctx context.Context, orgID, userID, role string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO memberships (organization_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, $3)`,
		orgID, userID, role,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAlreadyMember
		}
		return fmt.Errorf("store: create membership: %w", err)
	}
	return nil
}

// ListMembers returns an organization's members joined with profile info.
func (db *DB) ListMembers(ctx context.Context, orgID string) ([]model.Member, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT u.id::text, u.email, u.name, m.role, m.created_at
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1::uuid
		ORDER BY m.created_at`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list members: %w", err)
	}
	defer rows.Close()

	var members []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("store: scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// UpdateMemberRole changes a member's role. ErrNotFound if not a member.
func (db *DB) UpdateMemberRole(ctx context.Context, orgID, userID, role string) error {
	tag, err := db.Pool.Exec(ctx,
		`UPDATE memberships SET role = $3 WHERE organization_id = $1::uuid AND user_id = $2::uuid`,
		orgID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("store: update member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMembership removes a member. ErrNotFound if not a member.
func (db *DB) DeleteMembership(ctx context.Context, orgID, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM memberships WHERE organization_id = $1::uuid AND user_id = $2::uuid`,
		orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("store: delete membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountOwners returns the number of owners in an organization (to protect the last owner).
func (db *DB) CountOwners(ctx context.Context, orgID string) (int, error) {
	var n int
	err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE organization_id = $1::uuid AND role = 'owner'`,
		orgID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count owners: %w", err)
	}
	return n, nil
}
