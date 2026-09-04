package http

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/repositories"
	"backend/internal/interface/http/handlers"
	"backend/internal/interface/http/middleware"
	"backend/internal/interface/http/swagger"
	"backend/internal/usecase"
)

// SetupRouter configura os middlewares e inicializa todas as rotas da API.
// Recebe um EventPublisher (implementado pelo ChannelPool) para enfileirar eventos
// de forma segura sob concorrência, o WhatsAppManager (client da Evolution) e o prefixo
// de instância por-tenant. O endpoint /health é registrado no main.go, pois depende da
// conexão do banco e do broker para reportar o estado real da infra.
// secullumSvc é opcional: quando nil, a resolução de filial pelo aparelho da batida é
// pulada e a filial vem do nº de folha (ver usecase.BranchResolverService).
func SetupRouter(db *gorm.DB, publisher handlers.EventPublisher, whatsappMgr domain.WhatsAppManager, whatsappPrefix string, secullumSvc domain.SecullumService) *gin.Engine {
	router := gin.Default()

	// CORS de desenvolvimento (permite o painel de testes em outra porta).
	router.Use(middleware.DevCORS())

	// Repositórios
	tenantRepo := repositories.NewTenantRepository(db)
	staffRepo := repositories.NewStaffRepository(db)
	reportRepo := repositories.NewReportRepository(db)
	collaboratorRepo := repositories.NewCollaboratorRepository(db)
	userRepo := repositories.NewUserRepository(db)
	userTenantRepo := repositories.NewUserTenantRepository(db)
	occurrenceRepo := repositories.NewOccurrenceRepository(db)
	branchRepo := repositories.NewBranchRepository(db)
	warningRepo := repositories.NewWarningRepository(db)
	equipRepo := repositories.NewEquipmentRepository(db)
	punchRecordRepo := repositories.NewPunchRecordRepository(db)

	// Resolvedor de filial (aparelho da batida, com fallback pelo nº de folha).
	branchResolver := usecase.NewBranchResolverService(branchRepo)

	// Documentação (Swagger UI em /swagger, spec em /openapi.yaml)
	swagger.Register(router)

	// Handlers
	auditHandler := handlers.NewAuditHandler(publisher, userTenantRepo)
	tenantHandler := handlers.NewTenantHandler(tenantRepo, userTenantRepo, publisher)
	staffHandler := handlers.NewStaffHandler(staffRepo, userTenantRepo)
	settingsHandler := handlers.NewSettingsHandler(tenantRepo)
	reportHandler := handlers.NewReportHandler(reportRepo)
	collaboratorHandler := handlers.NewCollaboratorHandler(collaboratorRepo, tenantRepo, branchResolver, secullumSvc, punchRecordRepo)
	whatsappHandler := handlers.NewWhatsAppHandler(whatsappMgr, whatsappPrefix)
	userHandler := handlers.NewUserHandler(userRepo, userTenantRepo)
	monthlyReviewRepo := repositories.NewMonthlyReviewRepository(db)
	monthlyReviewSvc := usecase.NewMonthlyReviewService(occurrenceRepo, reportRepo, monthlyReviewRepo, tenantRepo)
	monthlyReviewHandler := handlers.NewMonthlyReviewHandler(monthlyReviewSvc, monthlyReviewRepo)
	occurrenceHandler := handlers.NewOccurrenceHandler(occurrenceRepo, collaboratorRepo, tenantRepo, userTenantRepo, branchResolver, secullumSvc, monthlyReviewRepo)
	treatmentRepo := repositories.NewTreatmentRepository(db)
	treatmentSvc := usecase.NewTreatmentService(occurrenceRepo, treatmentRepo, monthlyReviewRepo)
	treatmentHandler := handlers.NewTreatmentHandler(treatmentSvc, treatmentRepo, occurrenceRepo, userTenantRepo)
	branchHandler := handlers.NewBranchHandler(branchRepo, userTenantRepo)
	warningHandler := handlers.NewWarningHandler(warningRepo, collaboratorRepo, userTenantRepo)
	equipmentHandler := handlers.NewEquipmentHandler(equipRepo)

	// Login é a única rota de /api/v1 pública: sem token não há como obter um, e o
	// cadastro de novos usuários (register) passa a exigir um super admin autenticado.
	// O primeiro usuário (super admin) é criado via seed — ver
	// docs/05_Auth_Backend_Contract.md.
	publicV1 := router.Group("/api/v1")
	publicV1.POST("/auth/login", userHandler.Login)

	// Agrupamento de Rotas V1 (todas exigem "Authorization: Bearer <token>")
	v1 := router.Group("/api/v1")
	v1.Use(middleware.RequireAuth())
	{
		// Autenticação — cadastro de usuário é uma ação administrativa global, só o
		// super admin pode criar outros usuários.
		v1.POST("/auth/register", middleware.RequireSuperAdmin(), userHandler.Register)

		// Usuários — listar/excluir são ações globais (só super admin); ver/editar os
		// próprios dados é permitido ao dono da conta ou a um super admin.
		v1.GET("/users", middleware.RequireSuperAdmin(), userHandler.List)
		v1.GET("/users/:id", middleware.RequireSelfOrSuperAdmin("id"), userHandler.Get)
		v1.GET("/users/:id/tenants", middleware.RequireSelfOrSuperAdmin("id"), userHandler.ListTenants)
		v1.PUT("/users/:id/email", middleware.RequireSelfOrSuperAdmin("id"), userHandler.UpdateEmail)
		v1.PUT("/users/:id/password", middleware.RequireSelfOrSuperAdmin("id"), userHandler.UpdatePassword)
		v1.PATCH("/users/:id/activate", middleware.RequireSuperAdmin(), userHandler.Activate)
		v1.PATCH("/users/:id/deactivate", middleware.RequireSuperAdmin(), userHandler.Deactivate)
		v1.DELETE("/users/:id", middleware.RequireSuperAdmin(), userHandler.Delete)

		// Auditoria (o tenant_id vem no corpo; o acesso é checado dentro do handler)
		v1.POST("/audit/trigger", auditHandler.TriggerAudit)

		// Tenants — criar é ação administrativa global (só super admin); listar é
		// filtrado por vínculo dentro do handler.
		v1.GET("/tenants", tenantHandler.List)
		v1.POST("/tenants", middleware.RequireSuperAdmin(), tenantHandler.Create)
		v1.PUT("/tenants/:id", middleware.RequireSuperAdmin(), tenantHandler.Update)
		v1.PATCH("/tenants/:id/activate", middleware.RequireSuperAdmin(), tenantHandler.Activate)
		v1.PATCH("/tenants/:id/deactivate", middleware.RequireSuperAdmin(), tenantHandler.Deactivate)
		v1.DELETE("/tenants/:id", middleware.RequireSuperAdmin(), tenantHandler.Delete)

		// Gestão do vínculo usuário↔tenant (só super admin associa/desassocia).
		v1.POST("/tenants/:id/users", middleware.RequireSuperAdmin(), tenantHandler.AddUser)
		v1.DELETE("/tenants/:id/users/:userId", middleware.RequireSuperAdmin(), tenantHandler.RemoveUser)

		// Responsáveis (staff) — id próprio na rota, tenant só é conhecido após
		// carregar o registro; checagem de acesso feita dentro do handler.
		v1.PUT("/staffs/:staffId", staffHandler.Update)
		v1.DELETE("/staffs/:staffId", staffHandler.Delete)

		// Ocorrências — mesma convenção: o id da ocorrência é a chave, o tenant sai do
		// registro e o acesso é conferido dentro do handler.
		v1.PATCH("/occurrences/:occurrenceId/ignore", occurrenceHandler.Ignore)
		v1.GET("/occurrences/:occurrenceId/events", occurrenceHandler.Events)

		// Tratativa (Feature 4) — mesma convenção acima.
		v1.POST("/occurrences/:occurrenceId/treat", treatmentHandler.Treat)
		v1.GET("/occurrences/:occurrenceId/treatments", treatmentHandler.Treatments)
		v1.POST("/treatments/:treatmentId/undo", treatmentHandler.Undo)
		v1.GET("/attachments/:attachmentId/download", treatmentHandler.DownloadAttachment)

		// Filiais (aparelhos e nº de folha vinculados)
		v1.GET("/branches/:branchId", branchHandler.Get)
		v1.PUT("/branches/:branchId", branchHandler.Update)
		v1.DELETE("/branches/:branchId", branchHandler.Delete)
		v1.POST("/branches/:branchId/devices", branchHandler.AddDevice)
		v1.DELETE("/branches/:branchId/devices/:deviceId", branchHandler.RemoveDevice)
		v1.POST("/branches/:branchId/payroll-numbers", branchHandler.AddPayrollNumber)
		v1.DELETE("/branches/:branchId/payroll-numbers/:payrollNumberId", branchHandler.RemovePayrollNumber)

		// Advertências
		v1.GET("/warnings/:warningId", warningHandler.Get)
		v1.PUT("/warnings/:warningId", warningHandler.Update)
		v1.PATCH("/warnings/:warningId/status", warningHandler.UpdateStatus)
		v1.DELETE("/warnings/:warningId", warningHandler.Delete)

		// Recursos aninhados sob /tenants/:id — todos exigem vínculo com o tenant
		// (ou super admin). É este middleware que garante o isolamento: dados de um
		// tenant nunca vazam para quem não tem vínculo com ele.
		tenantScoped := v1.Group("/tenants/:id")
		tenantScoped.Use(middleware.RequireTenantAccess(userTenantRepo, "id"))
		{
			tenantScoped.GET("", tenantHandler.Get)
			tenantScoped.POST("/sync", tenantHandler.Sync)

			// Configurações do tenant
			tenantScoped.GET("/settings", settingsHandler.Get)
			tenantScoped.PUT("/settings", settingsHandler.Update)

			// Responsáveis (staff) do tenant
			tenantScoped.GET("/staffs", staffHandler.List)
			tenantScoped.POST("/staffs", staffHandler.Create)

			// Relatórios de auditoria (painel de consulta): /reports traz só a mais
			// recente de cada dia; /reports/history traz o histórico completo, inclusive
			// reauditorias do mesmo dia. Ambas aceitam ?start_date=&end_date=.
			tenantScoped.GET("/reports", reportHandler.List)
			tenantScoped.GET("/reports/history", reportHandler.History)

			// Ocorrências (máquina de estados). Substitui a leitura de "lista de
			// inconsistências por varredura": aqui cada ocorrência aparece UMA vez, com
			// o estado atual, já enriquecida com horário fixo e filial.
			tenantScoped.GET("/occurrences", occurrenceHandler.List)
			// Histórico de tratamento (Feature 1): eventos de TODAS as ocorrências do
			// tenant num período, com colaborador e tipo já embutidos.
			tenantScoped.GET("/occurrence-events", occurrenceHandler.TenantEvents)

			// Revisão mensal (Feature 3) — ?competencia=YYYY-MM em todas as cinco.
			tenantScoped.GET("/monthly-reviews", monthlyReviewHandler.Get)
			tenantScoped.PATCH("/monthly-reviews", monthlyReviewHandler.UpdateManualConditions)
			tenantScoped.POST("/monthly-reviews/close", monthlyReviewHandler.Close)
			tenantScoped.POST("/monthly-reviews/reopen", monthlyReviewHandler.Reopen)
			tenantScoped.GET("/monthly-reviews/export", monthlyReviewHandler.Export)

			// Filiais do tenant
			tenantScoped.GET("/branches", branchHandler.List)
			tenantScoped.POST("/branches", branchHandler.Create)

			// Equipamentos (relógios de ponto) sincronizados do tenant — somente leitura.
			tenantScoped.GET("/equipamentos", equipmentHandler.List)

			// Advertências do tenant
			tenantScoped.GET("/warnings", warningHandler.List)
			tenantScoped.POST("/warnings", warningHandler.Create)

			// Colaboradores sincronizados (espelho local do tenant) — só ativos
			tenantScoped.GET("/collaborators", collaboratorHandler.List)
			// Histórico completo (ativos + demitidos)
			tenantScoped.GET("/collaborators/history", collaboratorHandler.History)
			// Departamentos, funções e empresas distintos entre os colaboradores do
			// tenant — seletores de filtro (histórico, dashboards, ranking).
			tenantScoped.GET("/collaborators/filters", collaboratorHandler.Filters)
			// Autopreenchimento da tela de colaborador: horário fixo (Secullum) + filial.
			tenantScoped.GET("/collaborators/:secullumId/prefill", collaboratorHandler.Prefill)
			// Enriquecimento de equipamento/motivo por dia (cruzado com FonteDados na
			// auditoria) — ?start_date=&end_date= (ambos obrigatórios).
			tenantScoped.GET("/collaborators/:secullumId/punch-records", collaboratorHandler.PunchRecords)

			// WhatsApp (instância da Evolution API por tenant)
			tenantScoped.GET("/whatsapp/status", whatsappHandler.Status)
			tenantScoped.POST("/whatsapp/instance", whatsappHandler.Connect)
			tenantScoped.DELETE("/whatsapp/instance", whatsappHandler.Disconnect)

			// Usuários vinculados ao tenant
			tenantScoped.GET("/users", tenantHandler.ListUsers)
		}
	}

	return router
}
