package authz

import (
	"testing"

	"cairn/internal/model"
)

func TestCan(t *testing.T) {
	cases := []struct {
		role   string
		action Action
		want   bool
	}{
		{model.RoleOwner, ActionOrgDelete, true},
		{model.RoleAdmin, ActionOrgDelete, false},
		{model.RoleAdmin, ActionOrgUpdate, true},
		{model.RoleAdmin, ActionMemberInvite, true},
		{model.RoleMember, ActionMemberInvite, false},
		{model.RoleMember, ActionOrgView, true},
		{model.RoleGuest, ActionOrgView, true},
		{model.RoleGuest, ActionMemberInvite, false},
		{"bogus", ActionOrgView, false},
		{model.RoleOwner, Action("unknown:action"), false},
	}

	for _, c := range cases {
		if got := Can(c.role, c.action); got != c.want {
			t.Errorf("Can(%q, %q) = %v, want %v", c.role, c.action, got, c.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	valid := []string{model.RoleOwner, model.RoleAdmin, model.RoleMember, model.RoleGuest}
	for _, r := range valid {
		if !ValidRole(r) {
			t.Errorf("ValidRole(%q) = false, want true", r)
		}
	}
	if ValidRole("superadmin") {
		t.Error("ValidRole(\"superadmin\") = true, want false")
	}
}
