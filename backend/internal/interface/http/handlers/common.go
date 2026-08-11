package handlers

import (
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
