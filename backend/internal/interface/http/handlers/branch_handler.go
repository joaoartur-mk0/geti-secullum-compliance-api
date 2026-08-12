package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type BranchRequest struct {
	Name         string `json:"name" binding:"required"`
	ManagerName  string `json:"manager_name"`
	ManagerPhone string `json:"manager_phone"`
}

type BranchDeviceRequest struct {
	SecullumEquipID int    `json:"secullum_equip_id" binding:"required"`
	Label           string `json:"label"`
}

type BranchPayrollNumberRequest struct {
	Numero string `json:"numero" binding:"required"`
}

type BranchHandler struct {
	branchRepo     domain.BranchRepository
	userTenantRepo domain.UserTenantRepository
}

func NewBranchHandler(repo domain.BranchRepository, userTenantRepo domain.UserTenantRepository) *BranchHandler {
	return &BranchHandler{branchRepo: repo, userTenantRepo: userTenantRepo}
}

type branchDeviceResponse struct {
	ID              int    `json:"id"`
	BranchID        int    `json:"branch_id"`
	SecullumEquipID int    `json:"secullum_equip_id"`
	Label           string `json:"label"`
}

type branchPayrollNumberResponse struct {
	ID       int    `json:"id"`
	BranchID int    `json:"branch_id"`
	Numero   string `json:"numero"`
}

type branchResponse struct {
	ID             int                           `json:"id"`
	TenantID       int                           `json:"tenant_id"`
	Name           string                        `json:"name"`
	ManagerName    string                        `json:"manager_name"`
	ManagerPhone   string                        `json:"manager_phone"`
	Devices        []branchDeviceResponse        `json:"devices"`
	PayrollNumbers []branchPayrollNumberResponse `json:"payroll_numbers"`
}

func toBranchResponse(b domain.Branch) branchResponse {
	out := branchResponse{
		ID:             b.ID,
		TenantID:       b.TenantID,
		Name:           b.Name,
		ManagerName:    b.ManagerName,
		ManagerPhone:   b.ManagerPhone,
		Devices:        make([]branchDeviceResponse, 0, len(b.Devices)),
		PayrollNumbers: make([]branchPayrollNumberResponse, 0, len(b.PayrollNumbers)),
	}
	for _, d := range b.Devices {
		out.Devices = append(out.Devices, branchDeviceResponse{
			ID: d.ID, BranchID: d.BranchID, SecullumEquipID: d.SecullumEquipID, Label: d.Label,
		})
	}
	for _, p := range b.PayrollNumbers {
		out.PayrollNumbers = append(out.PayrollNumbers, branchPayrollNumberResponse{
			ID: p.ID, BranchID: p.BranchID, Numero: p.Numero,
		})
	}
	return out
}

// loadBranch carrega a filial e verifica o acesso do usuário ao tenant dono dela. As
// rotas de filial usam o id próprio (:branchId), então o tenant só é conhecido depois de
// ler o registro — mesma situação de staffs.
func (h *BranchHandler) loadBranch(c *gin.Context, op string) (*domain.Branch, error) {
	id, err := idParam(c, op, "branchId")
	if err != nil {
		return nil, err
	}
	branch, err := h.branchRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := ensureTenantAccess(c, h.userTenantRepo, op, branch.TenantID); err != nil {
		return nil, err
	}
	return branch, nil
}

// Create — POST /api/v1/tenants/:id/branches
func (h *BranchHandler) Create(c *gin.Context) {
	const op = "BranchHandler.Create"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req BranchRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	branch := &domain.Branch{
		TenantID:     tenantID,
		Name:         req.Name,
		ManagerName:  req.ManagerName,
		ManagerPhone: req.ManagerPhone,
	}
	if err := h.branchRepo.Create(branch); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "filial cadastrada com sucesso",
		"branch":  toBranchResponse(*branch),
	})
}

// List — GET /api/v1/tenants/:id/branches
func (h *BranchHandler) List(c *gin.Context) {
	const op = "BranchHandler.List"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	branches, err := h.branchRepo.ListByTenant(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]branchResponse, 0, len(branches))
	for _, b := range branches {
		out = append(out, toBranchResponse(b))
	}
	c.JSON(http.StatusOK, gin.H{"branches": out})
}

// Get — GET /api/v1/branches/:branchId
func (h *BranchHandler) Get(c *gin.Context) {
	const op = "BranchHandler.Get"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"branch": toBranchResponse(*branch)})
}

// Update — PUT /api/v1/branches/:branchId
func (h *BranchHandler) Update(c *gin.Context) {
	const op = "BranchHandler.Update"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req BranchRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	branch.Name = req.Name
	branch.ManagerName = req.ManagerName
	branch.ManagerPhone = req.ManagerPhone
	if err := h.branchRepo.Update(branch); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "filial atualizada com sucesso"})
}

// Delete — DELETE /api/v1/branches/:branchId
func (h *BranchHandler) Delete(c *gin.Context) {
	const op = "BranchHandler.Delete"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := h.branchRepo.Delete(branch.ID); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "filial excluída com sucesso"})
}

// AddDevice — POST /api/v1/branches/:branchId/devices
func (h *BranchHandler) AddDevice(c *gin.Context) {
	const op = "BranchHandler.AddDevice"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req BranchDeviceRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	device := &domain.BranchDevice{
		BranchID:        branch.ID,
		TenantID:        branch.TenantID,
		SecullumEquipID: req.SecullumEquipID,
		Label:           req.Label,
	}
	if err := h.branchRepo.AddDevice(device); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "aparelho vinculado com sucesso",
		"device": branchDeviceResponse{
			ID: device.ID, BranchID: device.BranchID,
			SecullumEquipID: device.SecullumEquipID, Label: device.Label,
		},
	})
}

// RemoveDevice — DELETE /api/v1/branches/:branchId/devices/:deviceId
func (h *BranchHandler) RemoveDevice(c *gin.Context) {
	const op = "BranchHandler.RemoveDevice"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	deviceID, err := idParam(c, op, "deviceId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// Só remove um aparelho que seja realmente desta filial: sem esta checagem, quem tem
	// acesso a um tenant conseguiria desvincular aparelhos de outro pelo id.
	if !ownsDevice(branch, deviceID) {
		httperr.Respond(c, domain.NewNotFound(op, "aparelho não encontrado nesta filial", nil))
		return
	}

	if err := h.branchRepo.RemoveDevice(deviceID); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "aparelho desvinculado com sucesso"})
}

// AddPayrollNumber — POST /api/v1/branches/:branchId/payroll-numbers
func (h *BranchHandler) AddPayrollNumber(c *gin.Context) {
	const op = "BranchHandler.AddPayrollNumber"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req BranchPayrollNumberRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	pn := &domain.BranchPayrollNumber{
		BranchID: branch.ID,
		TenantID: branch.TenantID,
		Numero:   req.Numero,
	}
	if err := h.branchRepo.AddPayrollNumber(pn); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":        "nº de folha vinculado com sucesso",
		"payroll_number": branchPayrollNumberResponse{ID: pn.ID, BranchID: pn.BranchID, Numero: pn.Numero},
	})
}

// RemovePayrollNumber — DELETE /api/v1/branches/:branchId/payroll-numbers/:payrollNumberId
func (h *BranchHandler) RemovePayrollNumber(c *gin.Context) {
	const op = "BranchHandler.RemovePayrollNumber"

	branch, err := h.loadBranch(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	pnID, err := idParam(c, op, "payrollNumberId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if !ownsPayrollNumber(branch, pnID) {
		httperr.Respond(c, domain.NewNotFound(op, "nº de folha não encontrado nesta filial", nil))
		return
	}

	if err := h.branchRepo.RemovePayrollNumber(pnID); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "nº de folha desvinculado com sucesso"})
}

func ownsDevice(branch *domain.Branch, deviceID int) bool {
	for _, d := range branch.Devices {
		if d.ID == deviceID {
			return true
		}
	}
	return false
}

func ownsPayrollNumber(branch *domain.Branch, pnID int) bool {
	for _, p := range branch.PayrollNumbers {
		if p.ID == pnID {
			return true
		}
	}
	return false
}
