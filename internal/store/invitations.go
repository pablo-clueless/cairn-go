package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

const invitationColumns = `id::text, organization_id::text, email, role, token_hash, invited_by::text, expires_at, accepted_at, created_at`

// CreateInvitation stores a pending invitation. ErrInvitePending if one already exists.
func (db *DB) CreateInvitation(ctx context.Context, orgID, email, role, tokenHash, invitedBy string, expiresAt time.Time) (*model.Invitation, error) {
	inv := &model.Invitation{}
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO invitations (organization_id, email, role, token_hash, invited_by, expires_at)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6)
		RETURNING `+invitationColumns,
		orgID, email, role, tokenHash, invitedBy, expiresAt,
	).Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrInvitePending
		}
		return nil, fmt.Errorf("store: create invitation: %w", err)
	}
	return inv, nil
}

// GetInvitationByTokenHash looks up an invitation by its hashed token. ErrNotFound if absent.
func (db *DB) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*model.Invitation, error) {
	inv := &model.Invitation{}
	err := db.Pool.QueryRow(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE token_hash = $1`, tokenHash,
	).Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get invitation: %w", err)
	}
	return inv, nil
}

// ListInvitations returns pending invitations for an organization.
func (db *DB) ListInvitations(ctx context.Context, orgID string) ([]model.Invitation, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+invitationColumns+` FROM invitations
		 WHERE organization_id = $1::uuid AND accepted_at IS NULL
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list invitations: %w", err)
	}
	defer rows.Close()

	var invs []model.Invitation
	for rows.Next() {
		var inv model.Invitation
		if err := rows.Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan invitation: %w", err)
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

// MarkInvitationAccepted stamps an invitation as accepted.
func (db *DB) MarkInvitationAccepted(ctx context.Context, id string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE invitations SET accepted_at = now() WHERE id = $1::uuid`, id,
	)
	if err != nil {
		return fmt.Errorf("store: mark invitation accepted: %w", err)
	}
	return nil
}

// DeleteInvitation removes an invitation scoped to its organization. ErrNotFound if absent.
func (db *DB) DeleteInvitation(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM invitations WHERE id = $1::uuid AND organization_id = $2::uuid`, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("store: delete invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
