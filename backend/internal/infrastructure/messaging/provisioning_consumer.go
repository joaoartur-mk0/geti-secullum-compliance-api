package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"backend/internal/usecase"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ProvisioningConsumer escuta a fila "tenant.provisioning" e sincroniza o espelho
// local de colaboradores (funcionários + jornadas) de um tenant a partir da Secullum.
// É disparado no cadastro do tenant e pode ser reacionado manualmente (endpoint de
// sync) sempre que o cadastro de funcionários mudar no lado da Secullum.
type ProvisioningConsumer struct {
	channel *amqp.Channel
	sync    *usecase.SynchronizerService
}

func NewProvisioningConsumer(ch *amqp.Channel, sync *usecase.SynchronizerService) *ProvisioningConsumer {
	return &ProvisioningConsumer{channel: ch, sync: sync}
}

func (c *ProvisioningConsumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		"tenant.provisioning",
		"",
		false, // Ack manual
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	log.Println("[*] Worker de Provisionamento aguardando mensagens...")

	for {
		select {
		case msg := <-msgs:
			c.processMessage(msg)
		case <-ctx.Done():
			log.Println("[*] Desligando Worker de Provisionamento graciosamente...")
			return nil
		}
	}
}

// reject rejeita a mensagem; em requeue aplica a mesma pausa do worker de auditoria
// (requeueBackoff) para não martelar a Secullum em caso de erro persistente.
func (c *ProvisioningConsumer) reject(msg amqp.Delivery, requeue bool, tenantID int, reason string) {
	log.Printf("[Provisioning][Rejeitando] Tenant %d: %s (requeue=%v)\n", tenantID, reason, requeue)
	if requeue {
		time.Sleep(requeueBackoff)
	}
	if err := msg.Nack(false, requeue); err != nil {
		log.Printf("[Provisioning][Erro] Falha ao rejeitar (Nack) mensagem do Tenant %d: %v\n", tenantID, err)
	}
}

func (c *ProvisioningConsumer) processMessage(msg amqp.Delivery) {
	var payload struct {
		TenantID int `json:"tenant_id"`
	}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		c.reject(msg, false, 0, fmt.Sprintf("payload inválido na fila: %v", err))
		return
	}

	log.Printf("-> Sincronizando colaboradores do Tenant %d...\n", payload.TenantID)

	n, err := c.sync.SyncTenant(payload.TenantID)
	if err != nil {
		// Falha externa (rede/token/tenant temporariamente indisponível) é transitória:
		// devolve à fila para nova tentativa.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao sincronizar colaboradores: %v", err))
		return
	}
	log.Printf("[OK] Tenant %d: %d colaborador(es) sincronizado(s).\n", payload.TenantID, n)

	log.Printf("-> Sincronizando equipamentos do Tenant %d...\n", payload.TenantID)
	m, err := c.sync.SyncEquipment(payload.TenantID)
	if err != nil {
		// Mesmo raciocínio: falha externa é transitória, devolve à fila. Os
		// colaboradores já sincronizados acima não são desfeitos — a reentrega repete
		// o upsert idempotente de colaboradores sem prejuízo.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao sincronizar equipamentos: %v", err))
		return
	}
	log.Printf("[OK] Tenant %d: %d equipamento(s) sincronizado(s).\n", payload.TenantID, m)

	if err := msg.Ack(false); err != nil {
		log.Printf("[Provisioning][Erro] Tenant %d: sincronização concluída, mas falha ao confirmar (Ack) mensagem: %v\n", payload.TenantID, err)
	}
}
