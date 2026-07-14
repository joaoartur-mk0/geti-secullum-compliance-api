package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"backend/internal/domain"
	"backend/internal/usecase"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AuditConsumer struct {
	channel     *amqp.Channel
	tenantRepo  domain.TenantRepository
	secullumSvc domain.SecullumService
	reportRepo  domain.ReportRepository
	auditorCore *usecase.AuditorService
}

func NewAuditConsumer(
	ch *amqp.Channel,
	tr domain.TenantRepository,
	ss domain.SecullumService,
	rr domain.ReportRepository,
	ac *usecase.AuditorService,
) *AuditConsumer {
	return &AuditConsumer{
		channel:     ch,
		tenantRepo:  tr,
		secullumSvc: ss,
		reportRepo:  rr,
		auditorCore: ac,
	}
}

// Start fica escutando a fila infinitamente em background
func (c *AuditConsumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		"audit.trigger", // Nome da fila
		"",              // Consumer tag
		false,           // Auto-Ack (falso = nós confirmamos manualmente quando terminar)
		false,           // Exclusive
		false,           // No-local
		false,           // No-wait
		nil,             // Args
	)
	if err != nil {
		return err
	}

	log.Println("[*] Worker de Auditoria aguardando mensagens...")

	// Loop infinito lendo o canal do Go
	for {
		select {
		case msg := <-msgs:
			c.processMessage(msg)
		case <-ctx.Done():
			log.Println("[*] Desligando Worker de Auditoria graciosamente...")
			return nil
		}
	}
}

// reject rejeita a mensagem e registra caso o próprio Nack falhe, para que
// nenhuma falha de broker passe despercebida (software auditável).
// requeue=true devolve a mensagem para nova tentativa; false a descarta.
func (c *AuditConsumer) reject(msg amqp.Delivery, requeue bool, tenantID int, reason string) {
	log.Printf("[Rejeitando] Tenant %d: %s (requeue=%v)\n", tenantID, reason, requeue)
	if err := msg.Nack(false, requeue); err != nil {
		log.Printf("[Erro] Falha ao rejeitar (Nack) mensagem do Tenant %d: %v\n", tenantID, err)
	}
}

func (c *AuditConsumer) processMessage(msg amqp.Delivery) {
	// 1. Decodifica o JSON que veio do Handler HTTP
	var payload struct {
		TenantID int `json:"tenant_id"`
	}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		// Payload malformado nunca ficará válido em nova tentativa: descarta (requeue=false).
		c.reject(msg, false, 0, fmt.Sprintf("payload inválido na fila: %v", err))
		return
	}

	log.Printf("-> Iniciando auditoria para o Tenant %d...\n", payload.TenantID)

	// 2. Busca as configurações do Tenant no Banco
	tenant, err := c.tenantRepo.GetByID(payload.TenantID)
	if err != nil || tenant == nil {
		// Tenant inexistente não se resolve com retry: descarta (requeue=false).
		c.reject(msg, false, payload.TenantID, fmt.Sprintf("tenant não encontrado: %v", err))
		return
	}

	// NOTA PARA O FUTURO: Aqui buscaríamos a lista de colaboradores (c.tenantRepo.GetCollaborators)
	// Para simplificar este passo, vamos assumir que temos um colaborador Mockado
	collab := &domain.Collaborator{ID: 1}

	// 3. Busca as batidas de Hoje e de Ontem na Secullum
	hoje := time.Now()
	ontem := hoje.AddDate(0, 0, -1)

	punchesHoje, err := c.secullumSvc.GetDailyPunches(tenant, hoje)
	if err != nil {
		// Falha externa (rede/token/rate-limit) é transitória: devolve à fila (requeue=true).
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao buscar batidas de hoje: %v", err))
		return
	}

	// As batidas de ontem alimentam apenas a regra de interjornada. Um erro aqui não
	// invalida a auditoria de hoje, mas NÃO é silenciado: registramos e seguimos sem elas.
	punchesOntem, err := c.secullumSvc.GetDailyPunches(tenant, ontem)
	if err != nil {
		log.Printf("[Aviso Secullum] Tenant %d: falha ao buscar batidas de ontem (interjornada será ignorada): %v\n", payload.TenantID, err)
		punchesOntem = nil
	}

	// Pega apenas a primeira batida para passar ao nosso Core (simplificação do loop)
	var hojePunch, ontemPunch *domain.DailyPunch
	if len(punchesHoje) > 0 {
		hojePunch = &punchesHoje[0]
	}
	if len(punchesOntem) > 0 {
		ontemPunch = &punchesOntem[0]
	}

	// 4. CHAMA O CÉREBRO DA APLICAÇÃO (Usecase)
	// isClosing=true: este worker grava um relatório consolidado, ou seja, é a
	// avaliação de FECHAMENTO. Quando a varredura intra-dia (cron) for implementada,
	// ela deverá chamar ProcessRules com isClosing=false.
	inconsistencies, err := c.auditorCore.ProcessRules(
		tenant.Settings,
		collab,
		hojePunch,
		ontemPunch,
		time.Now(),
		true,
	)
	if err != nil {
		// Erros de qualidade de dado (ex.: batida em formato inválido) não abortam a
		// auditoria nem são descartados: ficam registrados para investigação, e as
		// inconsistências detectadas com sucesso seguem para o relatório.
		log.Printf("[Aviso Auditoria] Tenant %d: regras retornaram erros de dado: %v\n", payload.TenantID, err)
	}

	// 5. Salva o Relatório no Banco de Dados
	report := &domain.Report{
		TenantID:        tenant.ID,
		Date:            ontem,
		DataGenerated:   time.Now(),
		Inconsistencies: inconsistencies,
	}
	if err := c.reportRepo.Save(report); err != nil {
		// Persistência falhou: não podemos perder a auditoria. Devolve à fila (requeue=true).
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao salvar relatório: %v", err))
		return
	}

	log.Printf("[OK] Auditoria concluída! %d infrações encontradas.\n", len(inconsistencies))

	// 6. Confirma (Ack) para o RabbitMQ apagar a mensagem da fila
	if err := msg.Ack(false); err != nil {
		// O relatório já foi salvo; o Ack falhou. A mensagem poderá ser reentregue e
		// gerar um relatório duplicado — registrado aqui para rastreabilidade.
		log.Printf("[Erro] Tenant %d: relatório salvo, mas falha ao confirmar (Ack) mensagem: %v\n", payload.TenantID, err)
	}
}
