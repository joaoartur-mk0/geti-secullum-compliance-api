package domain

// UserTenantRepository é o contrato para o vínculo N:N entre usuários e tenants.
// Um usuário pode acessar vários tenants, e um tenant pode ter vários usuários. É
// este vínculo que garante o isolamento dos dados entre tenants: fora de um super
// admin (User.IsSuperAdmin), todo acesso a um recurso de tenant passa por HasAccess.
type UserTenantRepository interface {
	// AddUserToTenant vincula o usuário ao tenant já com um papel — obrigatório e
	// validado no handler (role.Valid()), para forçar a escolha consciente na tela de
	// cadastro. Ver docs/08_Roles_And_Permissions_Contract.md §7.1.
	AddUserToTenant(userID uint, tenantID int, role Role) error
	RemoveUserFromTenant(userID uint, tenantID int) error
	HasAccess(userID uint, tenantID int) (bool, error)
	// GetRole devolve o papel do usuário naquele tenant. Sem vínculo, devolve erro
	// NotFound — "tem acesso" e "qual o papel" são perguntas diferentes (ver
	// middleware.RequireTenantRole: primeiro confere acesso, só depois o papel).
	GetRole(userID uint, tenantID int) (Role, error)
	// UpdateRole troca o papel de um vínculo já existente. NotFound se não houver vínculo.
	UpdateRole(userID uint, tenantID int, role Role) error
	ListTenantsForUser(userID uint) ([]*Tenant, error)
	// ListUsersForTenant devolve cada usuário do tenant JUNTO do papel que ele tem ali.
	ListUsersForTenant(tenantID int) ([]UserWithRole, error)
}
