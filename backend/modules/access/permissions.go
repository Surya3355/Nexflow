// Package access handles users, roles, login, sessions and permission checks.
package access

// Role is a staff role. Roles are fixed in code; users are assigned one.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleManager  Role = "manager"
	RoleEmployee Role = "employee"
)

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleManager, RoleEmployee:
		return true
	}
	return false
}

// Action is a permitted operation. One constant per row of spec §6.
type Action string

const (
	ActionViewAll        Action = "view_all"
	ActionAddRecord      Action = "add_record"
	ActionEditRecord     Action = "edit_record"
	ActionUploadDocument Action = "upload_document"
	ActionDropRecord     Action = "drop_record"
	ActionAssignCase     Action = "assign_case"
	ActionHardDelete     Action = "hard_delete"
	ActionRestoreArchive Action = "restore_archive"
	ActionManageConfig   Action = "manage_config"
	ActionManageUsers    Action = "manage_users"
	ActionViewReports    Action = "view_reports"
	// ActionCorrectPayment edits or deletes a recorded disbursement payment (spec 2026-11-25 §5.4).
	ActionCorrectPayment Action = "correct_payment"
)

// AllActions lists every action, in spec §6 order.
var AllActions = []Action{
	ActionViewAll, ActionAddRecord, ActionEditRecord, ActionUploadDocument,
	ActionDropRecord, ActionAssignCase, ActionHardDelete, ActionRestoreArchive,
	ActionManageConfig, ActionManageUsers, ActionViewReports, ActionCorrectPayment,
}

// matrix is spec §6 as data, so it can be compared with the spec row by row.
var matrix = map[Action]map[Role]bool{
	ActionViewAll:        {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionAddRecord:      {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionEditRecord:     {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionUploadDocument: {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionDropRecord:     {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionAssignCase:     {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionHardDelete:     {RoleAdmin: true, RoleManager: true, RoleEmployee: false},
	ActionRestoreArchive: {RoleAdmin: true, RoleManager: false, RoleEmployee: false},
	ActionManageConfig:   {RoleAdmin: true, RoleManager: false, RoleEmployee: false},
	ActionManageUsers:    {RoleAdmin: true, RoleManager: false, RoleEmployee: false},
	ActionViewReports:    {RoleAdmin: true, RoleManager: true, RoleEmployee: true},
	ActionCorrectPayment: {RoleAdmin: true, RoleManager: true, RoleEmployee: false},
}

// Can reports whether role may perform action. Unknown roles and actions are refused.
func Can(role Role, action Action) bool {
	return matrix[action][role]
}
