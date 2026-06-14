// Package authz centralizes role-based permission checks for organizations.
// Handlers ask "can this role perform this action?" rather than inlining
// ad-hoc role comparisons.
package authz

import "cairn/internal/model"

// Action is a permission-gated operation within an organization.
type Action string

const (
	ActionOrgView          Action = "org:view"
	ActionOrgUpdate        Action = "org:update"
	ActionOrgDelete        Action = "org:delete"
	ActionMemberInvite     Action = "member:invite"
	ActionMemberRemove     Action = "member:remove"
	ActionMemberRoleUpdate Action = "member:role:update"
)

// roleRank orders roles from least to most privileged.
var roleRank = map[string]int{
	model.RoleGuest:  0,
	model.RoleMember: 1,
	model.RoleAdmin:  2,
	model.RoleOwner:  3,
}

// minRank is the lowest role rank permitted to perform each action.
var minRank = map[Action]int{
	ActionOrgView:          roleRank[model.RoleGuest],
	ActionMemberInvite:     roleRank[model.RoleAdmin],
	ActionMemberRemove:     roleRank[model.RoleAdmin],
	ActionMemberRoleUpdate: roleRank[model.RoleAdmin],
	ActionOrgUpdate:        roleRank[model.RoleAdmin],
	ActionOrgDelete:        roleRank[model.RoleOwner],
}

// Can reports whether a role may perform an action.
func Can(role string, action Action) bool {
	r, ok := roleRank[role]
	if !ok {
		return false
	}
	min, ok := minRank[action]
	if !ok {
		return false
	}
	return r >= min
}

// ValidRole reports whether s is a known role.
func ValidRole(s string) bool {
	_, ok := roleRank[s]
	return ok
}
