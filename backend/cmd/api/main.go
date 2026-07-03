package main

import (
	"log"
	"net/http"
	"os"

	"backend/internal/infrastructure/database/models"

	appHttp "backend/internal/interface/http"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	log.Println("Iniciando o Sistema de Auditoria de Jornada...")

	// 1. Configuração e Conexão com PostgreSQL
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Fallback para ambiente de desenvolvimento local
		dsn = "host=localhost user=postgres password=postgres dbname=auditoria_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Falha ao conectar ao banco de dados PostgreSQL: %v", err)
	}
	log.Println("Conexão com PostgreSQL estabelecida com sucesso.")

	// Executa as migrações automáticas baseadas nas structs que definimos
	err = db.AutoMigrate(
		&models.Tenant{},
		&models.TenantSettings{},
		&models.Collaborator{},
		&models.CollaboratorSchedule{},
		&models.Staff{},
		&models.Report{},
	)
	if err != nil {
		log.Fatalf("Falha ao executar AutoMigrate do GORM: %v", err)
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

	// Configuração das filas essenciais da nossa arquitetura
	ch, err := rabbitConn.Channel()
	if err != nil {
		log.Fatalf("Falha ao abrir um canal no RabbitMQ: %v", err)
	}
	defer ch.Close()

	filas := []string{"audit.trigger", "audit.process", "notifications.whatsapp", "tenant.provisioning"}
	for _, fila := range filas {
		_, err = ch.QueueDeclare(
			fila,
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			log.Fatalf("Falha ao declarar a fila %s: %v", fila, err)
		}
	}
	log.Println("Filas do RabbitMQ declaradas com sucesso.")

	// 3. Inicialização do Gin Gonic
	// Definimos o Gin para usar o modo Release em produção via variável de ambiente
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// 4. Definição de Rotas Iniciais
	// Endpoint de verificação de integridade da infraestrutura (Conexão DB e Broker)
	router.GET("/health", func(c *gin.Context) {
		// Validação simples de ping no banco de dados
		sqlDB, err := db.DB()
		dbStatus := "up"
		if err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
		}

		// Validação do status da conexão AMQP
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

	router = appHttp.SetupRouter(db, ch)
	// 5. Inicialização do Servidor HTTP
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Servidor da API de Auditoria rodando na porta %s...", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Falha ao iniciar o servidor HTTP: %v", err)
	}
}
