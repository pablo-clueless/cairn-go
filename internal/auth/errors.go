package auth

import "errors"

var (
	// ErrInvalidCredentials is returned for a bad email/password pair.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrEmailTaken is returned when signing up with an existing email.
	ErrEmailTaken = errors.New("auth: email already in use")
	// ErrInvalidToken is returned for a malformed/expired/revoked token.
	ErrInvalidToken = errors.New("auth: invalid token")
)
