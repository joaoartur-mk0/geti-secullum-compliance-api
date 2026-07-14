package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
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
