package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
)

// isSuperAdmin lê a flag injetada por RequireAuth no contexto (false se ausente —
// nunca deveria faltar em rotas atrás de RequireAuth).
func isSuperAdmin(c *gin.Context) bool {
	v, ok := c.Get(ContextIsSuperAdminKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func currentUserID(c *gin.Context) uint {
	v, ok := c.Get(ContextUserIDKey)
	if !ok {
		return 0
	}
	id, _ := v.(uint)
	return id
}

func forbidden(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": gin.H{"code": string(domain.KindForbidden), "message": message},
	})
}

// RequireSuperAdmin restringe a rota a usuários com is_super_admin = true. Usado nas
// ações administrativas globais (criar tenant, cadastrar/excluir usuário).
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isSuperAdmin(c) {
			forbidden(c, "apenas o super admin pode realizar esta ação")
			return
		}
		c.Next()
	}
}

// RequireSelfOrSuperAdmin restringe a rota ao próprio usuário autenticado (comparando
// o parâmetro de rota informado com o id do token) ou a um super admin.
func RequireSelfOrSuperAdmin(param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSuperAdmin(c) {
			c.Next()
			return
		}
		id, err := strconv.ParseUint(c.Param(param), 10, 64)
		if err != nil || uint(id) != currentUserID(c) {
			forbidden(c, "você só pode acessar os seus próprios dados")
			return
		}
		c.Next()
	}
}

// RequireTenantAccess restringe a rota a super admins ou a usuários vinculados ao
// tenant indicado pelo parâmetro de rota (ex.: :id em /tenants/:id/...). É esta
// checagem que impede os dados de um tenant vazarem para usuários sem vínculo com
// ele — deve estar presente em toda rota que exponha dados de um tenant específico.
func RequireTenantAccess(repo domain.UserTenantRepository, param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSuperAdmin(c) {
			c.Next()
			return
		}

		tenantID, err := strconv.Atoi(c.Param(param))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{"code": string(domain.KindValidation), "message": "parâmetro de rota inválido"},
			})
			return
		}

		hasAccess, err := repo.HasAccess(currentUserID(c), tenantID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{"code": string(domain.KindInternal), "message": "falha ao verificar acesso ao tenant"},
			})
			return
		}
		if !hasAccess {
			forbidden(c, "você não tem acesso a este tenant")
			return
		}
		c.Next()
	}
}

// RequireTenantRole exige vínculo com o tenant da rota E papel mínimo — irmão de
// RequireTenantAccess (que continua sendo o piso "Diretoria" nas rotas de leitura, e
// segue existindo à parte porque acesso e papel mínimo são checagens distintas: primeiro
// SE tem acesso, só depois QUANTO). Super admin passa direto, como em toda checagem desta
// camada. Ver docs/08_Roles_And_Permissions_Contract.md §6.1.
func RequireTenantRole(repo domain.UserTenantRepository, param string, min domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSuperAdmin(c) {
			c.Next()
			return
		}

		tenantID, err := strconv.Atoi(c.Param(param))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{"code": string(domain.KindValidation), "message": "parâmetro de rota inválido"},
			})
			return
		}

		role, err := repo.GetRole(currentUserID(c), tenantID)
		if err != nil {
			// "Sem vínculo" (NotFound) vira 403; qualquer outro erro é falha real de
			// infraestrutura e deve aparecer como 500 — mesma distinção que HasAccess
			// já faz acima, para GetRole.
			var appErr *domain.AppError
			if errors.As(err, &appErr) && appErr.Kind == domain.KindNotFound {
				forbidden(c, "você não tem acesso a este tenant")
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{"code": string(domain.KindInternal), "message": "falha ao verificar perfil de acesso"},
			})
			return
		}
		if !role.Valid() {
			// Linha corrompida ou de uma versão futura do enum: nega tudo, nunca libera
			// por omissão.
			forbidden(c, "seu perfil de acesso não permite esta ação")
			return
		}
		if !role.AtLeast(min) {
			forbidden(c, "seu perfil de acesso não permite esta ação")
			return
		}

		c.Set(ContextRoleKey, role)
		c.Next()
	}
}
