package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type StaffRequest struct {
	Name    string `json:"name" binding:"required"`
	Celular string `json:"celular" binding:"required"`
}

type StaffHandler struct {
	staffRepo domain.StaffRepository
}

func NewStaffHandler(repo domain.StaffRepository) *StaffHandler {
	return &StaffHandler{staffRepo: repo}
}

type staffResponse struct {
	ID       int    `json:"id"`
	TenantID int    `json:"tenant_id"`
	Name     string `json:"name"`
	Celular  string `json:"celular"`
}

func toStaffResponse(s domain.Staff) staffResponse {
	return staffResponse{ID: s.ID, TenantID: s.TenantID, Name: s.Name, Celular: s.Celular}
}

// Create — POST /api/v1/tenants/:id/staffs
func (h *StaffHandler) Create(c *gin.Context) {
	const op = "StaffHandler.Create"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req StaffRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	staff := &domain.Staff{TenantID: tenantID, Name: req.Name, Celular: req.Celular}
	if err := h.staffRepo.Create(staff); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message": "responsável cadastrado com sucesso",
		"staff":   toStaffResponse(*staff),
	})
}

// List — GET /api/v1/tenants/:id/staffs
func (h *StaffHandler) List(c *gin.Context) {
	const op = "StaffHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	staffs, err := h.staffRepo.ListByTenant(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]staffResponse, 0, len(staffs))
	for _, s := range staffs {
		out = append(out, toStaffResponse(s))
	}
	c.JSON(http.StatusOK, gin.H{"staffs": out})
}

// Update — PUT /api/v1/staffs/:staffId
func (h *StaffHandler) Update(c *gin.Context) {
	const op = "StaffHandler.Update"

	id, err := idParam(c, op, "staffId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req StaffRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	staff := &domain.Staff{ID: id, Name: req.Name, Celular: req.Celular}
	if err := h.staffRepo.Update(staff); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "responsável atualizado com sucesso"})
}

// Delete — DELETE /api/v1/staffs/:staffId
func (h *StaffHandler) Delete(c *gin.Context) {
	const op = "StaffHandler.Delete"

	id, err := idParam(c, op, "staffId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.staffRepo.Delete(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "responsável excluído com sucesso"})
}
