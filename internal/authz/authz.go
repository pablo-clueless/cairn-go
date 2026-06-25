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

	// Work (spaces & issues)
	ActionWorkView     Action = "work:view"
	ActionSpaceCreate  Action = "space:create"
	ActionSpaceUpdate  Action = "space:update"
	ActionSpaceDelete  Action = "space:delete"
	ActionIssueCreate  Action = "issue:create"
	ActionIssueUpdate  Action = "issue:update"
	ActionIssueDelete  Action = "issue:delete"
	ActionSprintManage Action = "sprint:manage"
	ActionStatusManage Action = "status:manage"

	// Documents (space pages, live docs)
	ActionDocumentCreate Action = "document:create"
	ActionDocumentUpdate Action = "document:update"
	ActionDocumentDelete Action = "document:delete"

	// Comments (per-issue collaboration). Author-only edit/delete is enforced in
	// the work layer on top of these role gates.
	ActionCommentCreate Action = "comment:create"
	ActionCommentUpdate Action = "comment:update"
	ActionCommentDelete Action = "comment:delete"

	// Attachments (per-issue files). Uploader-or-admin delete is enforced in the
	// work layer on top of these role gates.
	ActionAttachmentCreate Action = "attachment:create"
	ActionAttachmentDelete Action = "attachment:delete"
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

	ActionWorkView:     roleRank[model.RoleGuest],
	ActionSpaceCreate:  roleRank[model.RoleAdmin],
	ActionSpaceUpdate:  roleRank[model.RoleAdmin],
	ActionSpaceDelete:  roleRank[model.RoleAdmin],
	ActionIssueCreate:  roleRank[model.RoleMember],
	ActionIssueUpdate:  roleRank[model.RoleMember],
	ActionIssueDelete:  roleRank[model.RoleAdmin],
	ActionSprintManage: roleRank[model.RoleMember],
	ActionStatusManage: roleRank[model.RoleAdmin],

	ActionDocumentCreate: roleRank[model.RoleMember],
	ActionDocumentUpdate: roleRank[model.RoleMember],
	ActionDocumentDelete: roleRank[model.RoleMember],

	ActionCommentCreate: roleRank[model.RoleMember],
	ActionCommentUpdate: roleRank[model.RoleMember],
	ActionCommentDelete: roleRank[model.RoleMember],

	ActionAttachmentCreate: roleRank[model.RoleMember],
	ActionAttachmentDelete: roleRank[model.RoleMember],
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
