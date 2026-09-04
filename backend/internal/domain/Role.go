package domain

// Role é o papel de acesso de um usuário DENTRO DE UM TENANT — ver
// docs/08_Roles_And_Permissions_Contract.md. Vive no vínculo (UserTenant), não no
// usuário: a mesma pessoa pode ser RH num cliente e Diretoria em outro.
//
// Os três papéis são ESTRITAMENTE ANINHADOS: Diretoria ⊂ Gestor ⊂ RH. Não existe
// permissão que um papel de baixo tenha e um de cima não — por isso a checagem é uma
// comparação de nível (AtLeast), não uma matriz de permissões.
type Role string

const (
	RoleDiretoria Role = "diretoria"
	RoleGestor    Role = "gestor"
	RoleRH        Role = "rh"
)

// Level ordena os papéis para a checagem de "papel mínimo". Super admin não entra aqui:
// é global (User.IsSuperAdmin) e passa por cima de qualquer nível — ver
// middleware.RequireTenantRole.
func (r Role) Level() int {
	switch r {
	case RoleRH:
		return 3
	case RoleGestor:
		return 2
	case RoleDiretoria:
		return 1
	}
	return 0 // papel desconhecido: nega tudo, nunca libera por omissão
}

// Valid indica se o valor é um dos três papéis conhecidos.
func (r Role) Valid() bool { return r.Level() > 0 }

// AtLeast responde se este papel satisfaz o papel mínimo exigido por uma rota.
func (r Role) AtLeast(min Role) bool { return r.Level() >= min.Level() }

// RoleSuperAdmin é o valor que as respostas de API usam para o papel de um super admin —
// não é um Role de verdade (não tem linha em user_tenants, não entra em Level()), é só o
// rótulo que o frontend lê num campo só, sem precisar cruzar com is_super_admin à parte.
const RoleSuperAdmin = "super_admin"

// UserWithRole é o usuário junto do papel que ele tem NAQUELE tenant — o mesmo usuário
// aparece com papéis diferentes em tenants diferentes (ver ListUsersForTenant).
type UserWithRole struct {
	User User
	Role Role
}
