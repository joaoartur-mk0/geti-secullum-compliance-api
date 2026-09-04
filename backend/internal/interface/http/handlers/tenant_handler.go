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
	"backend/internal/interface/http/middleware"
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
	tenantRepo     domain.TenantRepository
	userTenantRepo domain.UserTenantRepository
	publisher      EventPublisher
}

func NewTenantHandler(repo domain.TenantRepository, userTenantRepo domain.UserTenantRepository, publisher EventPublisher) *TenantHandler {
	return &TenantHandler{tenantRepo: repo, userTenantRepo: userTenantRepo, publisher: publisher}
}

// publishProvisioning enfileira o pedido de sincronização do tenant — colaboradores E
// equipamentos, ambos processados pelo mesmo worker (ver
// messaging.ProvisioningConsumer.processMessage). Usa um contexto próprio (não o da
// requisição HTTP) com um timeout curto, pois a publicação deve seguir mesmo que a
// resposta HTTP já tenha sido enviada, e falha ao publicar não desfaz o cadastro/gatilho
// já concluído: fica só registrada em log, já que o usuário ainda pode sincronizar depois
// via POST /tenants/:id/sync.
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

// tenantResponse é o DTO de saída (não expõe relacionamentos pesados). Role é omitido
// (vazio) fora dos contextos que resolvem o papel do usuário no tenant (login, GET
// /tenants) — ver docs/08_Roles_And_Permissions_Contract.md §4.
type tenantResponse struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	SecullumDatabaseID int    `json:"secullum_database_id"`
	Active             bool   `json:"active"`
	Role               string `json:"role,omitempty"`
}

func toTenantResponse(t *domain.Tenant) tenantResponse {
	return tenantResponse{
		ID:                 t.ID,
		Name:               t.Name,
		SecullumDatabaseID: t.SecullumDatabaseID,
		Active:             t.Active,
	}
}

// tenantResponseWithRole resolve e embute o papel do usuário `uid` no tenant `t` —
// "super_admin" para super admin (nenhuma linha em user_tenants necessária), o papel do
// vínculo para os demais. Papel não encontrado (nunca deveria acontecer para um tenant
// que ListTenantsForUser já filtrou) fica vazio em vez de quebrar a resposta inteira.
func tenantResponseWithRole(userTenantRepo domain.UserTenantRepository, t *domain.Tenant, uid uint, isSuperAdminUser bool) tenantResponse {
	out := toTenantResponse(t)
	if isSuperAdminUser {
		out.Role = domain.RoleSuperAdmin
		return out
	}
	if role, err := userTenantRepo.GetRole(uid, t.ID); err == nil {
		out.Role = string(role)
	}
	return out
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
// Reenfileira a sincronização de colaboradores e equipamentos sob demanda (ex.: após
// alterações no cadastro de funcionários/aparelhos na Secullum, ou para popular um tenant
// já existente que ainda não foi sincronizado). Botão "Ressincronizar" nas telas de
// Colaboradores e Equipamentos do painel.
func (h *TenantHandler) Sync(c *gin.Context) {
	const op = "TenantHandler.Sync"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	h.publishProvisioning(id)

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "sincronização de colaboradores e equipamentos enfileirada com sucesso",
		"tenant_id": id,
		"status":    "processing",
	})
}

// List — GET /api/v1/tenants?include_inactive=true
// Super admins veem todos os tenants; demais usuários só os tenants aos quais têm
// vínculo (nunca a lista completa — evita vazar a existência de outros clientes).
func (h *TenantHandler) List(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"

	var (
		tenants []*domain.Tenant
		err     error
	)

	isSuperAdminUser, _ := c.Get(middleware.ContextIsSuperAdminKey)
	superAdmin, _ := isSuperAdminUser.(bool)

	userID, _ := c.Get(middleware.ContextUserIDKey)
	uid, _ := userID.(uint)

	if superAdmin {
		tenants, err = h.tenantRepo.List(includeInactive)
	} else {
		tenants, err = h.userTenantRepo.ListTenantsForUser(uid)
		if err == nil && !includeInactive {
			active := make([]*domain.Tenant, 0, len(tenants))
			for _, t := range tenants {
				if t.Active {
					active = append(active, t)
				}
			}
			tenants = active
		}
	}

	if err != nil {
		httperr.Respond(c, err)
		return
	}

	// Papel do usuário ATIVO em cada tenant — é o que o AppShell usa quando o usuário
	// troca de cliente no seletor, sem precisar de um novo login (docs/08 §4.2).
	out := make([]tenantResponse, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, tenantResponseWithRole(h.userTenantRepo, t, uid, superAdmin))
	}
	c.JSON(http.StatusOK, gin.H{"tenants": out})
}

type addUserToTenantRequest struct {
	UserID uint `json:"user_id" binding:"required"`
	// Role é OBRIGATÓRIO — sem default aqui, mesmo a coluna tendo default 'rh': é esta
	// rota que força a escolha consciente do papel na tela de cadastro (ver
	// docs/08_Roles_And_Permissions_Contract.md §7.1). O default da coluna só existe
	// para preservar o acesso de vínculos criados ANTES desta coluna existir.
	Role string `json:"role" binding:"required"`
}

// AddUser — POST /api/v1/tenants/:id/users
// Vincula um usuário já existente ao tenant, com um papel (só super admin).
func (h *TenantHandler) AddUser(c *gin.Context) {
	const op = "TenantHandler.AddUser"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req addUserToTenantRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	role := domain.Role(req.Role)
	if !role.Valid() {
		httperr.Respond(c, domain.NewValidation(op, "papel inválido", nil).
			WithDetails("role deve ser rh, gestor ou diretoria"))
		return
	}

	if err := h.userTenantRepo.AddUserToTenant(req.UserID, tenantID, role); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message": "usuário vinculado ao tenant com sucesso",
		"user_id": req.UserID, "tenant_id": tenantID, "role": string(role),
	})
}

// UpdateUserRole — PATCH /api/v1/tenants/:id/users/:userId/role
// Troca o papel de um vínculo já existente (só super admin) — sem este endpoint, mudar
// alguém de papel exigiria desvincular e vincular de novo.
func (h *TenantHandler) UpdateUserRole(c *gin.Context) {
	const op = "TenantHandler.UpdateUserRole"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	userID, err := idParam(c, op, "userId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}
	role := domain.Role(req.Role)
	if !role.Valid() {
		httperr.Respond(c, domain.NewValidation(op, "papel inválido", nil).
			WithDetails("role deve ser rh, gestor ou diretoria"))
		return
	}

	if err := h.userTenantRepo.UpdateRole(uint(userID), tenantID, role); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "perfil de acesso atualizado",
		"user_id": userID, "tenant_id": tenantID, "role": string(role),
	})
}

// RemoveUser — DELETE /api/v1/tenants/:id/users/:userId
// Remove o vínculo de um usuário com o tenant (só super admin).
func (h *TenantHandler) RemoveUser(c *gin.Context) {
	const op = "TenantHandler.RemoveUser"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	userID, err := idParam(c, op, "userId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.userTenantRepo.RemoveUserFromTenant(uint(userID), tenantID); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "vínculo removido com sucesso"})
}

// ListUsers — GET /api/v1/tenants/:id/users
// Lista os usuários com acesso ao tenant (membros do próprio tenant ou super admin).
func (h *TenantHandler) ListUsers(c *gin.Context) {
	const op = "TenantHandler.ListUsers"

	tenantID, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	users, err := h.userTenantRepo.ListUsersForTenant(tenantID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]userWithRoleResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userWithRoleResponse{userResponse: toUserResponse(u.User), Role: string(u.Role)})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
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

// Activate — PATCH /api/v1/tenants/:id/activate
func (h *TenantHandler) Activate(c *gin.Context) {
	const op = "TenantHandler.Activate"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.tenantRepo.Activate(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tenant ativado com sucesso"})
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

// Delete — DELETE /api/v1/tenants/:id
// Apaga o tenant em cascata (settings, staffs, colaboradores, jornadas, relatórios,
// vínculos com usuários) — irreversível, perde o histórico de auditoria. Prefira
// Deactivate quando o objetivo é só suspender o acesso.
func (h *TenantHandler) Delete(c *gin.Context) {
	const op = "TenantHandler.Delete"

	id, err := idParam(c, op, "id")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.tenantRepo.Delete(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tenant apagado com sucesso"})
}
