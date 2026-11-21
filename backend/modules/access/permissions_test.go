package access

import "testing"

// Expected values copied from spec §6 (Permissions), one row per action.
// Columns: Admin, Manager, Employee.
func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		action                   Action
		admin, manager, employee bool
	}{
		{ActionViewAll, true, true, true},
		{ActionAddRecord, true, true, true},
		{ActionEditRecord, true, true, true},
		{ActionUploadDocument, true, true, true},
		{ActionDropRecord, true, true, true},
		{ActionAssignCase, true, true, true},
		{ActionHardDelete, true, true, false},
		{ActionRestoreArchive, true, false, false},
		{ActionManageConfig, true, false, false},
		{ActionManageUsers, true, false, false},
		{ActionViewReports, true, true, true},
	}
	for _, tc := range cases {
		want := map[Role]bool{RoleAdmin: tc.admin, RoleManager: tc.manager, RoleEmployee: tc.employee}
		for role, expected := range want {
			if got := Can(role, tc.action); got != expected {
				t.Errorf("Can(%s, %s) = %v, want %v", role, tc.action, got, expected)
			}
		}
	}
}

func TestSpecificRefusals(t *testing.T) {
	if Can(RoleEmployee, ActionHardDelete) {
		t.Error("employee must not hard-delete")
	}
	if Can(RoleManager, ActionRestoreArchive) {
		t.Error("manager must not restore archive")
	}
	if !Can(RoleEmployee, ActionAssignCase) {
		t.Error("employee must be able to assign (spec §6, round 8)")
	}
	if Can(RoleManager, ActionManageUsers) {
		t.Error("manager must not manage users")
	}
}

func TestUnknownRoleIsRefusedEverything(t *testing.T) {
	for _, action := range AllActions {
		if Can(Role("owner"), action) {
			t.Errorf("unknown role allowed %s", action)
		}
	}
}
