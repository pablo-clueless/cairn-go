package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

const userColumns = `id::text, email, name, coalesce(password_hash, '') AS password_hash, is_platform_admin, created_at, updated_at`

// CreateUser inserts a new user. Returns ErrEmailTaken on a duplicate email.
func (db *DB) CreateUser(ctx context.Context, email, name, passwordHash string) (*model.User, error) {
	u := &model.User{}
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		email, name, passwordHash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("store: create user: %w", err)
	}
	return u, nil
}

// GetUserByEmail looks up a user case-insensitively. Returns ErrNotFound if absent.
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	err := db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get user by email: %w", err)
	}
	return u, nil
}

// SetPlatformAdminByEmails flags the given emails as platform admins (idempotent).
// Used at startup to bootstrap operators from configuration.
func (db *DB) SetPlatformAdminByEmails(ctx context.Context, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET is_platform_admin = true WHERE lower(email) = ANY($1)`, emails,
	)
	if err != nil {
		return fmt.Errorf("store: set platform admins: %w", err)
	}
	return nil
}

// GetUserByID looks up a user by id. Returns ErrNotFound if absent.
func (db *DB) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1::uuid`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsPlatformAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get user by id: %w", err)
	}
	return u, nil
}
