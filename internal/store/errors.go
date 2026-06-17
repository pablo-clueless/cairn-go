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
	// ErrSpaceKeyTaken is returned when creating a space with a duplicate key.
	ErrSpaceKeyTaken = errors.New("store: space key already in use")
	// ErrStatusNameTaken is returned for a duplicate status name within a space.
	ErrStatusNameTaken = errors.New("store: status name already in use")
	// ErrStatusInUse is returned when deleting a status that still has issues.
	ErrStatusInUse = errors.New("store: status has issues and cannot be deleted")
)
