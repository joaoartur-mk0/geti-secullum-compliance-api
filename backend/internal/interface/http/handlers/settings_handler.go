package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

// UpdateSettingsRequest carrega as flags de regras, as severidades configuráveis e o
// horário da auditoria automática diária. As severidades aceitam "ALERTA" ou "CRITICO".
type UpdateSettingsRequest struct {
	Almoco       bool `json:"almoco"`
	Interjornada bool `json:"interjornada"`
	Hextras      bool `json:"hextras"`
	Esquecimento bool `json:"esquecimento"`

	AlmocoSeverity       string `json:"almoco_severity"`
	InterjornadaSeverity string `json:"interjornada_severity"`
	EsquecimentoSeverity string `json:"esquecimento_severity"`

	// Horario ("HH:MM") é o momento em que o agendador dispara sozinho o fechamento
	// automático de D-1 (ver usecase/scheduler.go). Vazio = sem agendamento.
	Horario string `json:"horario"`
}

type SettingsHandler struct {
	tenantRepo domain.TenantRepository
}

func NewSettingsHandler(repo domain.TenantRepository) *SettingsHandler {
	return &SettingsHandler{tenantRepo: repo}
}

type settingsResponse struct {
	Almoco               bool   `json:"almoco"`
	Interjornada         bool   `json:"interjornada"`
	Hextras              bool   `json:"hextras"`
	Esquecimento         bool   `json:"esquecimento"`
	AlmocoSeverity       string `json:"almoco_severity"`
	InterjornadaSeverity string `json:"interjornada_severity"`
	EsquecimentoSeverity string `json:"esquecimento_severity"`
	Horario              string `json:"horario"`
}

func toSettingsResponse(s *domain.TenantSettings) settingsResponse {
	return settingsResponse{
		Almoco:               s.Almoco,
		Interjornada:         s.Interjornada,
		Hextras:              s.Hextras,
		Esquecimento:         s.Esquecimento,
		AlmocoSeverity:       string(s.AlmocoSeverity),
		InterjornadaSeverity: string(s.InterjornadaSeverity),
		EsquecimentoSeverity: string(s.EsquecimentoSeverity),
		Horario:              s.Horario,
	}
}

// validSeverity aceita vazio (usa default) ou os valores conhecidos.
func validSeverity(s string) bool {
	switch domain.Severity(s) {
	case "", domain.SeverityAlert, domain.SeverityCritical:
		return true
	default:
		return false
	}
}

// validHorario aceita vazio (sem agendamento) ou "HH:MM".
func validHorario(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("15:04", s)
	return err == nil
}

// Get — GET /api/v1/tenants/:id/settings
func (h *SettingsHandler) Get(c *gin.Context) {
	const op = "SettingsHandler.Get"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	settings, err := h.tenantRepo.GetSettings(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": toSettingsResponse(settings)})
}

// Update — PUT /api/v1/tenants/:id/settings
func (h *SettingsHandler) Update(c *gin.Context) {
	const op = "SettingsHandler.Update"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req UpdateSettingsRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	// Valida as severidades informadas.
	for field, val := range map[string]string{
		"almoco_severity":       req.AlmocoSeverity,
		"interjornada_severity": req.InterjornadaSeverity,
		"esquecimento_severity": req.EsquecimentoSeverity,
	} {
		if !validSeverity(val) {
			httperr.Respond(c, domain.NewValidation(op, "severidade inválida", nil).
				WithDetails(field+" deve ser 'ALERTA' ou 'CRITICO'"))
			return
		}
	}
	if !validHorario(req.Horario) {
		httperr.Respond(c, domain.NewValidation(op, "horário inválido", nil).
			WithDetails("horario deve estar vazio ou no formato HH:MM"))
		return
	}

	settings := &domain.TenantSettings{
		Almoco:               req.Almoco,
		Interjornada:         req.Interjornada,
		Hextras:              req.Hextras,
		Esquecimento:         req.Esquecimento,
		AlmocoSeverity:       domain.Severity(req.AlmocoSeverity),
		InterjornadaSeverity: domain.Severity(req.InterjornadaSeverity),
		EsquecimentoSeverity: domain.Severity(req.EsquecimentoSeverity),
		Horario:              req.Horario,
	}
	if err := h.tenantRepo.UpdateSettings(tenantID, settings); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "configurações atualizadas com sucesso"})
}
