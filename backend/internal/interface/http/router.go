package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"

	"backend/internal/interface/http/handlers"
)

// SetupRouter configura os middlewares e inicializa todas as rotas da API
func SetupRouter(db *gorm.DB, rabbitCh *amqp.Channel) *gin.Engine {
	router := gin.Default()

	// Handler de Injeção de Dependências
	auditHandler := handlers.NewAuditHandler(rabbitCh)

	// Endpoint Global
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "API de Auditoria Operacional"})
	})

	// Agrupamento de Rotas V1
	v1 := router.Group("/api/v1")
	{
		v1.POST("/audit/trigger", auditHandler.TriggerAudit)
	}

	return router
}
