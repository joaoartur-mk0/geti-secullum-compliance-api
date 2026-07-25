package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"backend/internal/domain"

	amqp "github.com/rabbitmq/amqp091-go"
)

// NotificationConsumer escuta a fila `notifications.whatsapp` e entrega cada
// mensagem ao gestor (staff) via Evolution API. É o Worker desacoplado descrito em
// docs/00_Automation_Engineering_Documentation.md, seção 5.4: o motor de auditoria
// só publica o alerta na fila, sem esperar a latência da API externa de mensageria.
type NotificationConsumer struct {
	channel     *amqp.Channel
	notifierSvc domain.NotificationService
}

func NewNotificationConsumer(ch *amqp.Channel, notifierSvc domain.NotificationService) *NotificationConsumer {
	return &NotificationConsumer{channel: ch, notifierSvc: notifierSvc}
}

func (c *NotificationConsumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		"notifications.whatsapp",
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

	log.Println("[*] Worker de Notificações (WhatsApp) aguardando mensagens...")

	for {
		select {
		case msg := <-msgs:
			c.processMessage(msg)
		case <-ctx.Done():
			log.Println("[*] Desligando Worker de Notificações graciosamente...")
			return nil
		}
	}
}

// reject segue o mesmo padrão dos demais workers: requeue=true aplica a pausa
// (requeueBackoff) antes de devolver a mensagem, para não martelar a Evolution API
// em caso de falha persistente (ex.: instância desconectada do WhatsApp).
func (c *NotificationConsumer) reject(msg amqp.Delivery, requeue bool, tenantID int, reason string) {
	log.Printf("[Notificação][Rejeitando] Tenant %d: %s (requeue=%v)\n", tenantID, reason, requeue)
	if requeue {
		time.Sleep(requeueBackoff)
	}
	if err := msg.Nack(false, requeue); err != nil {
		log.Printf("[Notificação][Erro] Falha ao rejeitar (Nack) mensagem do Tenant %d: %v\n", tenantID, err)
	}
}

func (c *NotificationConsumer) processMessage(msg amqp.Delivery) {
	var payload domain.WhatsAppNotification
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		// Payload malformado nunca ficará válido em nova tentativa: descarta (requeue=false).
		c.reject(msg, false, 0, fmt.Sprintf("payload inválido na fila: %v", err))
		return
	}

	if payload.Number == "" || payload.Message == "" {
		c.reject(msg, false, payload.TenantID, "número ou mensagem vazios")
		return
	}

	if err := c.notifierSvc.SendText(payload.Number, payload.Message); err != nil {
		// Falha na API externa (rede/instância desconectada/rate-limit) é transitória:
		// devolve à fila para retry.
		c.reject(msg, true, payload.TenantID, fmt.Sprintf("falha ao enviar via Evolution API: %v", err))
		return
	}

	log.Printf("[OK] Notificação enviada ao staff %d do tenant %d.\n", payload.StaffID, payload.TenantID)

	if err := msg.Ack(false); err != nil {
		log.Printf("[Notificação][Erro] Tenant %d: mensagem enviada, mas falha ao confirmar (Ack): %v\n", payload.TenantID, err)
	}
}
