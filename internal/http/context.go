package http

import (
	"context"

	"cairn/internal/model"
)

type contextKey int

const (
	userContextKey contextKey = iota
	orgContextKey
)

// withUser returns a copy of ctx carrying the authenticated user.
func withUser(ctx context.Context, u *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// userFromContext retrieves the authenticated user, if any.
func userFromContext(ctx context.Context) (*model.User, bool) {
	u, ok := ctx.Value(userContextKey).(*model.User)
	return u, ok
}

// orgScope carries the resolved organization and the caller's role within it.
type orgScope struct {
	Org  *model.Organization
	Role string
}

// withOrg returns a copy of ctx carrying the resolved org scope.
func withOrg(ctx context.Context, org *model.Organization, role string) context.Context {
	return context.WithValue(ctx, orgContextKey, orgScope{Org: org, Role: role})
}

// orgFromContext retrieves the resolved org scope, if any.
func orgFromContext(ctx context.Context) (orgScope, bool) {
	s, ok := ctx.Value(orgContextKey).(orgScope)
	return s, ok
}
