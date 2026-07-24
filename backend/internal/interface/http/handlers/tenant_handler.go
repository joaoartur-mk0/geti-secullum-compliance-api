package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type CreateTenantRequest struct {
	Name               string `json:"name" binding:"required"`
	SecullumDatabaseID int    `json:"secullum_database_id" binding:"required"`
	StaffName          string `json:"staff_name" binding:"required"`
	StaffContact       string `json:"staff_contact" binding:"required"`
}

type UpdateTenantRequest struct {
	Name               string `json:"name" binding:"required"`
	SecullumDatabaseID int    `json:"secullum_database_id" binding:"required"`
}

type TenantHandler struct {
	tenantRepo domain.TenantRepository
	publisher  EventPublisher
}

func NewTenantHandler(repo domain.TenantRepository, publisher EventPublisher) *TenantHandler {
	return &TenantHandler{tenantRepo: repo, publisher: publisher}
}

// publishProvisioning enfileira o pedido de sincronização de colaboradores do tenant.
// Usa um contexto próprio (não o da requisição HTTP) com um timeout curto, pois a
// publicação deve seguir mesmo que a resposta HTTP já tenha sido enviada, e falha ao
// publicar não desfaz o cadastro/gatilho já concluído: fica só registrada em log,
// já que o usuário ainda pode sincronizar depois via POST /tenants/:id/sync.
func (h *TenantHandler) publishProvisioning(tenantID int) {
	payload, err := json.Marshal(map[string]int{"tenant_id": tenantID})
	if err != nil {
		log.Printf("[TenantHandler] Falha ao serializar evento de provisionamento do tenant %d: %v", tenantID, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.publisher.Publish(ctx, "tenant.provisioning", payload); err != nil {
		log.Printf("[TenantHandler] Falha ao enfileirar provisionamento do tenant %d: %v", tenantID, err)
	}
}

func toDomainTenant(req CreateTenantRequest) *domain.Tenant {
	return &domain.Tenant{
		Name:               req.Name,
		SecullumDatabaseID: req.SecullumDatabaseID,
		// settings tudo false por padrão (zero-value do bool).
		Settings: &domain.TenantSettings{},
		// separação: os campos "staff_*" viram um Staff dentro do Tenant.
		Staffs: []domain.Staff{
			{Name: req.StaffName, Celular: req.StaffContact},
		},
	}
}

// tenantResponse é o DTO de saída (não expõe relacionamentos pesados).
type tenantResponse struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	SecullumDatabaseID int    `json:"secullum_database_id"`
	Active             bool   `json:"active"`
}

func toTenantResponse(t *domain.Tenant) tenantResponse {
	return tenantResponse{
		ID:                 t.ID,
		Name:               t.Name,
		SecullumDatabaseID: t.SecullumDatabaseID,
		Active:             t.Active,
	}
}

// Create — POST /api/v1/tenants
func (h *TenantHandler) Create(c *gin.Context) {
	const op = "TenantHandler.Create"

	var req CreateTenantRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	tenant := toDomainTenant(req)
	if err := h.tenantRepo.Save(tenant); err != nil {
		httperr.Respond(c, err)
		return
	}

	// Dispara a sincronização de colaboradores (assíncrona, via fila
	// tenant.provisioning) para que a auditoria já tenha dados reais para trabalhar.
	h.publishProvisioning(tenant.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "tenant cadastrado com sucesso",
		"tenant":  toTenantResponse(tenant),
	})
}

// Sync — POST /api/v1/tenants/:id/sync
// Reenfileira a sincronização de colaboradores sob demanda (ex.: após alterações no
// cadastro de funcionários na Secullum, ou para popular colaboradores de um tenant
// já existente que ainda não foi sincronizado).
func (h *TenantHandler) Sync(c *gin.Context) {
	const op = "TenantHandler.Sync"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	h.publishProvisioning(id)

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "sincronização de colaboradores enfileirada com sucesso",
		"tenant_id": id,
		"status":    "processing",
	})
}

// List — GET /api/v1/tenants?include_inactive=true
func (h *TenantHandler) List(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"

	tenants, err := h.tenantRepo.List(includeInactive)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]tenantResponse, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, toTenantResponse(t))
	}
	c.JSON(http.StatusOK, gin.H{"tenants": out})
}

// Get — GET /api/v1/tenants/:id
func (h *TenantHandler) Get(c *gin.Context) {
	const op = "TenantHandler.Get"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	tenant, err := h.tenantRepo.GetByID(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": toTenantResponse(tenant)})
}

// Update — PUT /api/v1/tenants/:id
func (h *TenantHandler) Update(c *gin.Context) {
	const op = "TenantHandler.Update"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req UpdateTenantRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	tenant := &domain.Tenant{ID: id, Name: req.Name, SecullumDatabaseID: req.SecullumDatabaseID}
	if err := h.tenantRepo.Update(tenant); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tenant atualizado com sucesso"})
}

// Deactivate — PATCH /api/v1/tenants/:id/deactivate
func (h *TenantHandler) Deactivate(c *gin.Context) {
	const op = "TenantHandler.Deactivate"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.tenantRepo.Deactivate(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tenant desativado com sucesso"})
}
