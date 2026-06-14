package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

// CreateUserSSO inserts a user without a local password (federated login).
func (db *DB) CreateUserSSO(ctx context.Context, email, name string) (*model.User, error) {
	u := &model.User{}
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name)
		VALUES ($1, $2)
		RETURNING `+userColumns,
		email, name,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("store: create sso user: %w", err)
	}
	return u, nil
}

// GetUserByIdentity returns the user linked to a provider identity. ErrNotFound if absent.
func (db *DB) GetUserByIdentity(ctx context.Context, provider, providerUserID string) (*model.User, error) {
	u := &model.User{}
	err := db.Pool.QueryRow(ctx, `
		SELECT `+prefixedUserColumns("u")+`
		FROM user_identities i
		JOIN users u ON u.id = i.user_id
		WHERE i.provider = $1 AND i.provider_user_id = $2`,
		provider, providerUserID,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get user by identity: %w", err)
	}
	return u, nil
}

// LinkIdentity associates a provider identity with a user. Idempotent on the
// (provider, provider_user_id) unique constraint.
func (db *DB) LinkIdentity(ctx context.Context, userID, provider, providerUserID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, provider_user_id)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		userID, provider, providerUserID,
	)
	if err != nil {
		return fmt.Errorf("store: link identity: %w", err)
	}
	return nil
}

// prefixedUserColumns mirrors userColumns but qualified with a table alias.
func prefixedUserColumns(alias string) string {
	return alias + ".id::text, " + alias + ".email, " + alias + ".name, " +
		"coalesce(" + alias + ".password_hash, '') AS password_hash, " +
		alias + ".created_at, " + alias + ".updated_at"
}
