// Package middleware reúne middlewares HTTP do Gin.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DevCORS libera CORS para desenvolvimento (qualquer origem), permitindo que o painel
// de testes (servido em outra porta) chame a API. Responde o preflight OPTIONS com 204.
//
// ATENÇÃO: é permissivo de propósito, para desenvolvimento. Em produção, restrinja as
// origens permitidas.
func DevCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
