package governance

type Role string

const (
	RoleAccountant       Role = "ACCOUNTANT"
	RoleSeniorAccountant Role = "SENIOR_ACCOUNTANT"
	RoleFinanceManager   Role = "FINANCE_MANAGER"
	RoleController       Role = "CONTROLLER"
	RoleCFO              Role = "CFO"
)

type PermissionSet map[Permission]bool

type RolePermissions struct {
	Role        Role
	Permissions PermissionSet
}

func HasPermission(roles []Role, permissions PermissionSet, required Permission) bool {
	for _, role := range roles {
		if permissions != nil && permissions[required] {
			return true
		}
		_ = role
	}
	return false
}
