package store

import "errors"

var (
	// ErrNotFound is returned when a row does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrEmailTaken is returned when creating a user with a duplicate email.
	ErrEmailTaken = errors.New("store: email already in use")
	// ErrAlreadyMember is returned when a user is already a member of an org.
	ErrAlreadyMember = errors.New("store: already a member")
	// ErrInvitePending is returned when a pending invite already exists for an email.
	ErrInvitePending = errors.New("store: invitation already pending")
)
