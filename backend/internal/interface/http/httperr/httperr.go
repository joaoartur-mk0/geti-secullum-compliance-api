// Package httperr traduz erros da aplicação (domain.AppError) em respostas HTTP
// padronizadas para a interface web, registrando o contexto completo no log.
package httperr

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
)

// statusByKind mapeia a categoria do erro para o status HTTP.
var statusByKind = map[domain.ErrorKind]int{
	domain.KindValidation: http.StatusBadRequest,
	domain.KindNotFound:   http.StatusNotFound,
	domain.KindConflict:   http.StatusConflict,
	domain.KindInternal:   http.StatusInternalServerError,
}

// Respond escreve a resposta de erro padronizada e loga o contexto.
//
// Resposta (JSON):
//
//	{ "error": { "code": "NOT_FOUND", "message": "...", "details": "..." } }
//
// Erros que não sejam *domain.AppError são tratados como 500 (internos), sem vazar
// detalhes ao cliente, mas com log completo para investigação.
func Respond(c *gin.Context, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		status, ok := statusByKind[appErr.Kind]
		if !ok {
			status = http.StatusInternalServerError
		}

		// Log com o rastro completo (Op + Kind + erro subjacente).
		log.Printf("[HTTP %d] %s", status, appErr.Error())

		body := gin.H{"code": string(appErr.Kind), "message": appErr.Message}
		if appErr.Details != "" {
			body["details"] = appErr.Details
		}
		c.JSON(status, gin.H{"error": body})
		return
	}

	// Erro não estruturado: não expõe detalhes, mas registra tudo.
	log.Printf("[HTTP 500] erro não tratado: %v", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{"code": string(domain.KindInternal), "message": "erro interno inesperado"},
	})
}
