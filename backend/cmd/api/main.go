package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Ajuste "seu_projeto" para o nome real do seu módulo Go
	"backend/internal/infrastructure/database/models"
	"backend/internal/infrastructure/database/repositories"
	"backend/internal/infrastructure/messaging"
	"backend/internal/infrastructure/secullum"
	appHttp "backend/internal/interface/http"
	"backend/internal/usecase"
)

func main() {
	log.Println("Iniciando o Sistema de Auditoria de Jornada...")

	// 1. Configuração e Conexão com PostgreSQL
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=auditoria_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatalf("Falha ao conectar ao PostgreSQL: %v", err)
	}
	log.Println("Conexão com PostgreSQL estabelecida com sucesso.")

	err = db.AutoMigrate(
		&models.Tenant{},
		&models.TenantSettings{},
		&models.Collaborator{},
		&models.CollaboratorSchedule{},
		&models.Staff{},
		&models.Report{},
	)
	if err != nil {
		log.Fatalf("Falha no AutoMigrate do GORM: %v", err)
	}
	log.Println("Migração do banco de dados concluída.")

	// 2. Configuração e Conexão com RabbitMQ
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("Falha ao conectar ao RabbitMQ: %v", err)
	}
	defer rabbitConn.Close()
	log.Println("Conexão com RabbitMQ estabelecida com sucesso.")

	ch, err := rabbitConn.Channel()
	if err != nil {
		log.Fatalf("Falha ao abrir canal no RabbitMQ: %v", err)
	}
	defer ch.Close()

	filas := []string{"audit.trigger", "audit.process", "notifications.whatsapp", "tenant.provisioning"}
	for _, fila := range filas {
		_, err = ch.QueueDeclare(fila, true, false, false, false, nil)
		if err != nil {
			log.Fatalf("Falha ao declarar a fila %s: %v", fila, err)
		}
	}
	log.Println("Filas do RabbitMQ declaradas com sucesso.")

	// =====================================================================
	// 3. INJEÇÃO DE DEPENDÊNCIAS (WIRING)
	// =====================================================================

	// Repositórios (Mapeiam o DB para o Domínio)
	tenantRepo := repositories.NewTenantRepository(db)
	reportRepo := repositories.NewReportRepository(db)
	collabRepo := repositories.NewCollaboratorRepository(db)

	// Serviços Externos (Client HTTP da API Secullum).
	// Credenciais GLOBAIS (mesma para todos os tenants), vindas de env. O client cuida
	// da renovação do token (1h) e do rate limit (100 req/min) automaticamente.
	secullumBaseURL := os.Getenv("SECULLUM_API_URL")
	if secullumBaseURL == "" {
		secullumBaseURL = "https://pontowebintegracaoexterna.secullum.com.br"
	}
	secullumSvc := secullum.NewSecullumClient(secullum.Config{
		BaseURL:              secullumBaseURL,
		AuthURL:              os.Getenv("SECULLUM_AUTH_URL"),
		Username:             os.Getenv("SECULLUM_USERNAME"),
		Password:             os.Getenv("SECULLUM_PASSWORD"),
		StaticToken:          os.Getenv("SECULLUM_API_TOKEN"), // opcional (testes)
		MaxRequestsPerMinute: 100,
	})

	// Motor de Regras (O Cérebro)
	auditorCore := usecase.NewAuditorService()

	// Sincronizador de colaboradores: busca o espelho de funcionários/jornadas na
	// Secullum e o persiste localmente, alimentando o worker de auditoria com dados
	// reais (em vez do colaborador mockado usado nas fases iniciais do projeto).
	syncService := usecase.NewSynchronizerService(tenantRepo, collabRepo, secullumSvc)

	// Pool de canais para publicação HTTP (canais AMQP não são seguros para uso
	// concorrente; cada requisição empresta um canal exclusivo do pool).
	publisherPool, err := messaging.NewChannelPool(rabbitConn, 5)
	if err != nil {
		log.Fatalf("Falha ao criar o pool de canais de publicação: %v", err)
	}
	defer publisherPool.Close()

	// =====================================================================
	// 4. INICIALIZAÇÃO DOS WORKERS (CONSUMERS DO RABBITMQ)
	// =====================================================================
	auditConsumer := messaging.NewAuditConsumer(ch, tenantRepo, collabRepo, secullumSvc, reportRepo, auditorCore)

	// A palavra reservada 'go' faz esta função rodar em background.
	// Assim, ela fica a escutar a fila infinitamente sem parar o servidor web.
	go func() {
		log.Println("Iniciando Worker de Auditoria em background...")
		if err := auditConsumer.Start(context.Background()); err != nil {
			log.Fatalf("Falha crítica no Worker de Auditoria: %v", err)
		}
	}()

	// Canal dedicado para o worker de provisionamento (canais AMQP não são seguros
	// para uso concorrente entre goroutines; cada worker tem o seu próprio).
	provisioningCh, err := rabbitConn.Channel()
	if err != nil {
		log.Fatalf("Falha ao abrir canal de provisionamento no RabbitMQ: %v", err)
	}
	defer provisioningCh.Close()

	provisioningConsumer := messaging.NewProvisioningConsumer(provisioningCh, syncService)
	go func() {
		log.Println("Iniciando Worker de Provisionamento em background...")
		if err := provisioningConsumer.Start(context.Background()); err != nil {
			log.Fatalf("Falha crítica no Worker de Provisionamento: %v", err)
		}
	}()

	// =====================================================================
	// 5. INICIALIZAÇÃO DO SERVIDOR HTTP (GIN GONIC)
	// =====================================================================
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := appHttp.SetupRouter(db, publisherPool)

	// Endpoint de verificação de integridade da infraestrutura (Conexão DB e Broker).
	// Registrado aqui porque depende da conexão do banco e do broker.
	router.GET("/health", func(c *gin.Context) {
		dbStatus := "up"
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
		}

		rabbitStatus := "up"
		if rabbitConn.IsClosed() {
			rabbitStatus = "down"
		}

		statusHTTP := http.StatusOK
		if dbStatus == "down" || rabbitStatus == "down" {
			statusHTTP = http.StatusServiceUnavailable
		}

		c.JSON(statusHTTP, gin.H{
			"status":   "ok",
			"database": dbStatus,
			"rabbitmq": rabbitStatus,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Servidor da API de Auditoria a rodar na porta %s...", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Falha ao iniciar o servidor HTTP: %v", err)
	}
}
