package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

// CreateRefreshToken persists a hashed refresh token.
func (db *DB) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, userAgent string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1::uuid, $2, $3, $4)`,
		userID, tokenHash, expiresAt, userAgent,
	)
	if err != nil {
		return fmt.Errorf("store: create refresh token: %w", err)
	}
	return nil
}

// GetRefreshToken fetches a refresh token by its hash. Returns ErrNotFound if absent.
func (db *DB) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	t := &model.RefreshToken{}
	err := db.Pool.QueryRow(ctx, `
		SELECT id::text, user_id::text, token_hash, expires_at, revoked_at, coalesce(user_agent, ''), created_at
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.UserAgent, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get refresh token: %w", err)
	}
	return t, nil
}

// RevokeRefreshToken marks a single token as revoked (idempotent).
func (db *DB) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("store: revoke refresh token: %w", err)
	}
	return nil
}
