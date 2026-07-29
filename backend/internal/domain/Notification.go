package domain

import "fmt"

// WhatsAppNotification é o payload publicado na fila `notifications.whatsapp` pelo
// motor de auditoria (Worker 2) e consumido pelo worker de notificações, que o
// entrega ao gestor via Evolution API. TenantID/StaffID não fazem parte do contrato
// da Evolution — viajam junto apenas para rastreabilidade/log no worker consumidor.
type WhatsAppNotification struct {
	TenantID int    `json:"tenant_id"`
	StaffID  int    `json:"staff_id"`
	Instance string `json:"instance"` // instância de WhatsApp do tenant que fará o envio
	Number   string `json:"number"`
	Message  string `json:"message"`
}

// NotificationService abstrai o envio de mensagens de WhatsApp (implementado pelo
// client da Evolution API), para que o worker consumidor não conheça detalhes de
// transporte HTTP. O envio é por instância (uma por tenant).
type NotificationService interface {
	SendText(instance string, number string, message string) error
}

// WhatsAppInstance é o estado da instância de WhatsApp de um tenant na Evolution.
// QRCode vem preenchido (data URI base64) quando há um QR a escanear; fica vazio
// quando a instância já está conectada.
type WhatsAppInstance struct {
	QRCode    string `json:"qrcode,omitempty"`
	Connected bool   `json:"connected"`
	State     string `json:"state"`
}

// WhatsAppManager abstrai a gerência da instância de WhatsApp de um tenant (criar/
// conectar, consultar estado e desconectar), implementada pelo client da Evolution.
type WhatsAppManager interface {
	CreateInstance(instance string) (*WhatsAppInstance, error)
	ConnectionState(instance string) (*WhatsAppInstance, error)
	DeleteInstance(instance string) error
}

// WhatsAppInstanceName deriva o nome da instância de um tenant a partir do prefixo
// global (env EVOLUTION_INSTANCE_PREFIX) e do id do tenant. Ex.: "tenant" + 3 =>
// "tenant-3". É a fonte única desse formato, usada tanto no envio quanto na gerência.
func WhatsAppInstanceName(prefix string, tenantID int) string {
	if prefix == "" {
		prefix = "tenant"
	}
	return fmt.Sprintf("%s-%d", prefix, tenantID)
}
