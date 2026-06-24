package model

import "time"

// User is an authenticated person. PasswordHash is never serialized.
type User struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	PasswordHash    string    `json:"-"`
	IsPlatformAdmin bool      `json:"is_platform_admin"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RefreshToken is a persisted, hashed refresh credential used to mint new
// access tokens. Only the hash is stored; the raw value lives in a cookie.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent string
	CreatedAt time.Time
}

// Active reports whether the token can still be used.
func (t RefreshToken) Active(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

// PasswordResetToken backs the forgot/reset-password flow. Only the hash is
// stored; the raw token is emailed to the user and never persisted.
type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// Active reports whether the token can still be redeemed.
func (t PasswordResetToken) Active(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}
