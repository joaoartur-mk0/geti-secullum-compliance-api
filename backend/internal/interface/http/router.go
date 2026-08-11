package http

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/repositories"
	"backend/internal/interface/http/handlers"
	"backend/internal/interface/http/middleware"
	"backend/internal/interface/http/swagger"
)

// SetupRouter configura os middlewares e inicializa todas as rotas da API.
// Recebe um EventPublisher (implementado pelo ChannelPool) para enfileirar eventos
// de forma segura sob concorrência, o WhatsAppManager (client da Evolution) e o prefixo
// de instância por-tenant. O endpoint /health é registrado no main.go, pois depende da
// conexão do banco e do broker para reportar o estado real da infra.
func SetupRouter(db *gorm.DB, publisher handlers.EventPublisher, whatsappMgr domain.WhatsAppManager, whatsappPrefix string) *gin.Engine {
	router := gin.Default()

	// CORS de desenvolvimento (permite o painel de testes em outra porta).
	router.Use(middleware.DevCORS())

	// Repositórios
	tenantRepo := repositories.NewTenantRepository(db)
	staffRepo := repositories.NewStaffRepository(db)
	reportRepo := repositories.NewReportRepository(db)
	collaboratorRepo := repositories.NewCollaboratorRepository(db)
	userRepo := repositories.NewUserRepository(db)

	// Documentação (Swagger UI em /swagger, spec em /openapi.yaml)
	swagger.Register(router)

	// Handlers
	auditHandler := handlers.NewAuditHandler(publisher)
	tenantHandler := handlers.NewTenantHandler(tenantRepo, publisher)
	staffHandler := handlers.NewStaffHandler(staffRepo)
	settingsHandler := handlers.NewSettingsHandler(tenantRepo)
	reportHandler := handlers.NewReportHandler(reportRepo)
	collaboratorHandler := handlers.NewCollaboratorHandler(collaboratorRepo)
	whatsappHandler := handlers.NewWhatsAppHandler(whatsappMgr, whatsappPrefix)
	userHandler := handlers.NewUserHandler(userRepo)

	// Login é a única rota de /api/v1 pública: sem token não há como obter um, e o
	// cadastro de novos usuários (register) passa a exigir um usuário já autenticado.
	// O primeiro usuário (super admin) é criado via seed — ver
	// docs/05_Auth_Backend_Contract.md.
	publicV1 := router.Group("/api/v1")
	publicV1.POST("/auth/login", userHandler.Login)

	// Agrupamento de Rotas V1 (todas exigem "Authorization: Bearer <token>")
	v1 := router.Group("/api/v1")
	v1.Use(middleware.RequireAuth())
	{
		// Autenticação (cadastro de usuário exige um usuário já logado)
		v1.POST("/auth/register", userHandler.Register)

		// Usuários
		v1.GET("/users", userHandler.List)
		v1.GET("/users/:id", userHandler.Get)
		v1.PUT("/users/:id/email", userHandler.UpdateEmail)
		v1.PUT("/users/:id/password", userHandler.UpdatePassword)
		v1.DELETE("/users/:id", userHandler.Delete)

		// Auditoria
		v1.POST("/audit/trigger", auditHandler.TriggerAudit)

		// Tenants (CRUD + desativação)
		v1.GET("/tenants", tenantHandler.List)
		v1.POST("/tenants", tenantHandler.Create)
		v1.GET("/tenants/:id", tenantHandler.Get)
		v1.PUT("/tenants/:id", tenantHandler.Update)
		v1.PATCH("/tenants/:id/deactivate", tenantHandler.Deactivate)
		v1.POST("/tenants/:id/sync", tenantHandler.Sync)

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

		// Colaboradores sincronizados (espelho local do tenant)
		v1.GET("/tenants/:id/collaborators", collaboratorHandler.List)

		// WhatsApp (instância da Evolution API por tenant)
		v1.GET("/tenants/:id/whatsapp/status", whatsappHandler.Status)
		v1.POST("/tenants/:id/whatsapp/instance", whatsappHandler.Connect)
		v1.DELETE("/tenants/:id/whatsapp/instance", whatsappHandler.Disconnect)
	}

	return router
}
