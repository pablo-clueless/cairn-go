package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

// CreatePasswordResetToken persists a hashed, single-use password-reset token.
func (db *DB) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("store: create password reset token: %w", err)
	}
	return nil
}

// GetPasswordResetToken fetches a reset token by its hash. Returns ErrNotFound if absent.
func (db *DB) GetPasswordResetToken(ctx context.Context, tokenHash string) (*model.PasswordResetToken, error) {
	t := &model.PasswordResetToken{}
	err := db.Pool.QueryRow(ctx, `
		SELECT id::text, user_id::text, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get password reset token: %w", err)
	}
	return t, nil
}

// MarkPasswordResetTokenUsed marks a token as consumed (idempotent).
func (db *DB) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = now() WHERE token_hash = $1 AND used_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("store: mark password reset token used: %w", err)
	}
	return nil
}

// UpdateUserPassword sets a new bcrypt hash for a user.
func (db *DB) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1::uuid`,
		userID, passwordHash,
	)
	if err != nil {
		return fmt.Errorf("store: update user password: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokensForUser revokes every active refresh token for a user.
// Used after a password reset to invalidate existing sessions.
func (db *DB) RevokeAllRefreshTokensForUser(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("store: revoke all refresh tokens: %w", err)
	}
	return nil
}
