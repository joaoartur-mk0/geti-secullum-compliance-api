package http

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/infrastructure/database/repositories"
	"backend/internal/interface/http/handlers"
	"backend/internal/interface/http/swagger"
)

// SetupRouter configura os middlewares e inicializa todas as rotas da API.
// Recebe um EventPublisher (implementado pelo ChannelPool) para enfileirar eventos
// de forma segura sob concorrência. O endpoint /health é registrado no main.go,
// pois depende da conexão do banco e do broker para reportar o estado real da infra.
func SetupRouter(db *gorm.DB, publisher handlers.EventPublisher) *gin.Engine {
	router := gin.Default()

	// Repositórios
	tenantRepo := repositories.NewTenantRepository(db)
	staffRepo := repositories.NewStaffRepository(db)
	reportRepo := repositories.NewReportRepository(db)

	// Documentação (Swagger UI em /swagger, spec em /openapi.yaml)
	swagger.Register(router)

	// Handlers
	auditHandler := handlers.NewAuditHandler(publisher)
	tenantHandler := handlers.NewTenantHandler(tenantRepo)
	staffHandler := handlers.NewStaffHandler(staffRepo)
	settingsHandler := handlers.NewSettingsHandler(tenantRepo)
	reportHandler := handlers.NewReportHandler(reportRepo)

	// Agrupamento de Rotas V1
	v1 := router.Group("/api/v1")
	{
		// Auditoria
		v1.POST("/audit/trigger", auditHandler.TriggerAudit)

		// Tenants (CRUD + desativação)
		v1.GET("/tenants", tenantHandler.List)
		v1.POST("/tenants", tenantHandler.Create)
		v1.GET("/tenants/:id", tenantHandler.Get)
		v1.PUT("/tenants/:id", tenantHandler.Update)
		v1.PATCH("/tenants/:id/deactivate", tenantHandler.Deactivate)

		// Configurações do tenant
		v1.GET("/tenants/:id/settings", settingsHandler.Get)
		v1.PUT("/tenants/:id/settings", settingsHandler.Update)

		// Responsáveis (staff) do tenant
		v1.GET("/tenants/:id/staffs", staffHandler.List)
		v1.POST("/tenants/:id/staffs", staffHandler.Create)
		v1.PUT("/staffs/:staffId", staffHandler.Update)
		v1.DELETE("/staffs/:staffId", staffHandler.Delete)

		// Relatórios de auditoria (painel de consulta)
		v1.GET("/tenants/:id/reports", reportHandler.List)
	}

	return router
}
