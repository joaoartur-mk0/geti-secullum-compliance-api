package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"backend/internal/auth"
)

// Chaves usadas para guardar a identidade do usuário autenticado no contexto do Gin.
const (
	ContextUserIDKey    = "auth_user_id"
	ContextUserEmailKey = "auth_user_email"
)

// RequireAuth exige um token JWT válido no header "Authorization: Bearer <token>".
// Em caso de sucesso, injeta o id e o e-mail do usuário autenticado no contexto (chaves
// ContextUserIDKey/ContextUserEmailKey) para os handlers seguintes. Em caso de falha,
// interrompe a cadeia com 401 antes de chegar ao handler.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "Bearer "

		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			unauthorized(c, "token de autenticação ausente")
			return
		}

		tokenString := strings.TrimPrefix(header, prefix)
		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			unauthorized(c, "token de autenticação inválido ou expirado")
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserEmailKey, claims.Email)
		c.Next()
	}
}

func unauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{"code": "UNAUTHORIZED", "message": message},
	})
}
