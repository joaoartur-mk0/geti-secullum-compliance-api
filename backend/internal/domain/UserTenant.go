package domain

// UserTenantRepository é o contrato para o vínculo N:N entre usuários e tenants.
// Um usuário pode acessar vários tenants, e um tenant pode ter vários usuários. É
// este vínculo que garante o isolamento dos dados entre tenants: fora de um super
// admin (User.IsSuperAdmin), todo acesso a um recurso de tenant passa por HasAccess.
type UserTenantRepository interface {
	AddUserToTenant(userID uint, tenantID int) error
	RemoveUserFromTenant(userID uint, tenantID int) error
	HasAccess(userID uint, tenantID int) (bool, error)
	ListTenantsForUser(userID uint) ([]*Tenant, error)
	ListUsersForTenant(tenantID int) ([]User, error)
}
