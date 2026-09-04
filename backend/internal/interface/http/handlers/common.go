package handlers

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/middleware"
)

// bindJSON faz o parse do corpo JSON e devolve um erro de validação estruturado
// (com os detalhes do validator) quando a entrada é inválida.
func bindJSON(c *gin.Context, op string, req interface{}) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return domain.NewValidation(op, "corpo da requisição inválido", err).WithDetails(err.Error())
	}
	return nil
}

// idParam extrai um parâmetro de rota inteiro (ex.: :id) com erro de validação
// estruturado quando não é um número.
func idParam(c *gin.Context, op, name string) (int, error) {
	raw := c.Param(name)
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.NewValidation(op, "parâmetro de rota inválido", err).
			WithDetails(name + " deve ser um número inteiro")
	}
	return id, nil
}

// actorUserID devolve o id do usuário autenticado, para registrar QUEM tomou uma decisão
// (ignorar uma ocorrência, emitir uma advertência). Devolve nil se não houver identidade
// no contexto — as transições automáticas do worker não têm autor.
func actorUserID(c *gin.Context) *int {
	v, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		return nil
	}
	uid, ok := v.(uint)
	if !ok || uid == 0 {
		return nil
	}
	id := int(uid)
	return &id
}

// ensureTenantAccess garante que o usuário autenticado (via contexto, injetado pelo
// RequireAuth) seja super admin ou tenha vínculo com o tenant informado. Usado nos
// handlers cujo recurso não carrega o id do tenant diretamente no parâmetro de rota
// (ex.: staffs, cujo tenant só é conhecido depois de carregar o registro) — nesses
// casos o RequireTenantAccess do router não se aplica.
func ensureTenantAccess(c *gin.Context, repo domain.UserTenantRepository, op string, tenantID int) error {
	if v, ok := c.Get(middleware.ContextIsSuperAdminKey); ok {
		if isSuperAdmin, _ := v.(bool); isSuperAdmin {
			return nil
		}
	}

	userID, _ := c.Get(middleware.ContextUserIDKey)
	uid, _ := userID.(uint)

	hasAccess, err := repo.HasAccess(uid, tenantID)
	if err != nil {
		return err
	}
	if !hasAccess {
		return domain.NewForbidden(op, "você não tem acesso a este tenant", nil)
	}
	return nil
}

// requireRole confere papel mínimo para um tenant descoberto DENTRO do handler (ex.:
// /warnings/:warningId, cujo tenant só se conhece depois de carregar a advertência) —
// mesmo motivo de ensureTenantAccess existir separado de middleware.RequireTenantAccess.
// Substitui ensureTenantAccess (que só confere "tem acesso", piso Diretoria) nas rotas
// que exigem mais que leitura — ver docs/08_Roles_And_Permissions_Contract.md §6.2.
func requireRole(c *gin.Context, repo domain.UserTenantRepository, op string, tenantID int, min domain.Role) error {
	if v, ok := c.Get(middleware.ContextIsSuperAdminKey); ok {
		if isSuperAdmin, _ := v.(bool); isSuperAdmin {
			return nil
		}
	}

	userID, _ := c.Get(middleware.ContextUserIDKey)
	uid, _ := userID.(uint)

	role, err := repo.GetRole(uid, tenantID)
	if err != nil {
		// "Sem vínculo" (NotFound) vira 403 amigável; qualquer outro erro (falha real de
		// banco) segue como está — RequireTenantAccess, no mesmo pacote, já faz essa
		// distinção, e mascarar um 500 de infraestrutura como "sem permissão" esconde o
		// problema real de quem for investigar depois.
		var appErr *domain.AppError
		if errors.As(err, &appErr) && appErr.Kind == domain.KindNotFound {
			return domain.NewForbidden(op, "você não tem acesso a este tenant", nil)
		}
		return err
	}
	if !role.Valid() || !role.AtLeast(min) {
		return domain.NewForbidden(op, "seu perfil de acesso não permite esta ação", nil)
	}
	return nil
}
