package handlers

import (
	"net/http"

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
}

func NewTenantHandler(repo domain.TenantRepository) *TenantHandler {
	return &TenantHandler{tenantRepo: repo}
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

	c.JSON(http.StatusCreated, gin.H{
		"message": "tenant cadastrado com sucesso",
		"tenant":  toTenantResponse(tenant),
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
