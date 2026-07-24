package domain

// WhatsAppNotification é o payload publicado na fila `notifications.whatsapp` pelo
// motor de auditoria (Worker 2) e consumido pelo worker de notificações, que o
// entrega ao gestor via Evolution API. TenantID/StaffID não fazem parte do contrato
// da Evolution — viajam junto apenas para rastreabilidade/log no worker consumidor.
type WhatsAppNotification struct {
	TenantID int    `json:"tenant_id"`
	StaffID  int    `json:"staff_id"`
	Number   string `json:"number"`
	Message  string `json:"message"`
}

// NotificationService abstrai o envio de mensagens de WhatsApp (implementado pelo
// client da Evolution API), para que o worker consumidor não conheça detalhes de
// transporte HTTP.
type NotificationService interface {
	SendText(number string, message string) error
}
