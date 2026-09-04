package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/auth"
	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
)

type UserHandler struct {
	userRepo       domain.UserRepository
	userTenantRepo domain.UserTenantRepository
}

func NewUserHandler(repo domain.UserRepository, userTenantRepo domain.UserTenantRepository) *UserHandler {
	return &UserHandler{userRepo: repo, userTenantRepo: userTenantRepo}
}

type registerRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type updateEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type updatePasswordRequest struct {
	Password string `json:"password" binding:"required,min=8"`
}

type userResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	Active       bool   `json:"active"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{ID: u.ID, Name: u.Name, Email: u.Email, IsSuperAdmin: u.IsSuperAdmin, Active: u.Active}
}

// userWithRoleResponse é userResponse mais o papel do usuário NUM tenant específico —
// usado só em GET /tenants/:id/users (docs/08 §7.4), onde "quem tem acesso e com que
// papel" é a pergunta da tela.
type userWithRoleResponse struct {
	userResponse
	Role string `json:"role"`
}

// userIDParam extrai o :id da rota como uint (o domínio de User usa gorm.Model,
// cujo ID nunca é negativo).
func userIDParam(c *gin.Context, op string) (uint, error) {
	id, err := idParam(c, op, "id")
	if err != nil {
		return 0, err
	}
	if id < 0 {
		return 0, domain.NewValidation(op, "parâmetro de rota inválido", nil).WithDetails("id deve ser positivo")
	}
	return uint(id), nil
}

// Register — POST /api/v1/auth/register
func (h *UserHandler) Register(c *gin.Context) {
	const op = "UserHandler.Register"

	var req registerRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao gerar hash da senha", err))
		return
	}

	user := &domain.User{Name: req.Name, Email: req.Email, Password: hashed}
	if err := h.userRepo.Save(user); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "usuário cadastrado com sucesso",
		"user":    toUserResponse(*user),
	})
}

// Login — POST /api/v1/auth/login
func (h *UserHandler) Login(c *gin.Context) {
	const op = "UserHandler.Login"

	var req loginRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	user, err := h.userRepo.GetByEmail(req.Email)
	if err != nil {
		httperr.Respond(c, domain.NewValidation(op, "credenciais inválidas", nil))
		return
	}

	if err := auth.CheckPassword(user.Password, req.Password); err != nil {
		httperr.Respond(c, domain.NewValidation(op, "credenciais inválidas", nil))
		return
	}

	if !user.Active {
		httperr.Respond(c, domain.NewValidation(op, "usuário desativado", nil))
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.IsSuperAdmin)
	if err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao gerar token", err))
		return
	}

	// Tenants aos quais o usuário tem acesso, para o frontend montar o seletor de
	// tenant sem precisar de uma segunda chamada logo após o login. Cada um já vem com
	// o papel do usuário NAQUELE tenant (docs/08 §4.1) — super admin vê "super_admin" em
	// todos, sem precisar cruzar com is_super_admin em cada tela.
	tenants, err := h.userTenantRepo.ListTenantsForUser(user.ID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	tenantIDs := make([]int, 0, len(tenants))
	tenantsOut := make([]tenantLoginResponse, 0, len(tenants))
	for _, t := range tenants {
		tenantIDs = append(tenantIDs, t.ID)
		role := domain.RoleSuperAdmin
		if !user.IsSuperAdmin {
			r, err := h.userTenantRepo.GetRole(user.ID, t.ID)
			if err != nil {
				continue // vínculo inconsistente: não expõe um tenant sem papel resolvido
			}
			role = string(r)
		}
		tenantsOut = append(tenantsOut, tenantLoginResponse{ID: t.ID, Name: t.Name, Role: role})
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  toUserResponse(*user),
		// tenant_ids é mantido durante a transição — sai quando o frontend migrar para
		// ler só `tenants` (docs/08 §4.1).
		"tenant_ids": tenantIDs,
		"tenants":    tenantsOut,
	})
}

// tenantLoginResponse é o formato mínimo de tenant exposto no login — id, nome e o
// papel do usuário NAQUELE tenant. Ver docs/08_Roles_And_Permissions_Contract.md §4.1.
type tenantLoginResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// UpdateSuperAdmin — PATCH /api/v1/users/:id/super-admin
// Promove/rebaixa um usuário a super admin (só super admin). Resolve a pendência
// registrada no doc de auth: hoje só dá para virar super admin via seed ou UPDATE
// direto no banco.
//
// Duas regras (docs/08 §7.3): um super admin não pode rebaixar A SI MESMO — deixaria o
// sistema sem nenhum super admin se fosse o único —, e a alteração só vale no PRÓXIMO
// LOGIN do alvo, porque is_super_admin já viajou dentro do JWT emitido e não há
// revogação de token hoje.
func (h *UserHandler) UpdateSuperAdmin(c *gin.Context) {
	const op = "UserHandler.UpdateSuperAdmin"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req struct {
		IsSuperAdmin bool `json:"is_super_admin"`
	}
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	if uid := actorUserID(c); !req.IsSuperAdmin && uid != nil && id == uint(*uid) {
		httperr.Respond(c, domain.NewValidation(op, "não é possível rebaixar a si mesmo", nil).
			WithDetails("peça a outro super admin para alterar seu próprio perfil"))
		return
	}

	if err := h.userRepo.SetSuperAdmin(id, req.IsSuperAdmin); err != nil {
		httperr.Respond(c, err)
		return
	}

	user, err := h.userRepo.GetByID(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "usuário atualizado — a mudança vale a partir do próximo login",
		"user":    toUserResponse(*user),
	})
}

// Get — GET /api/v1/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	const op = "UserHandler.Get"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	user, err := h.userRepo.GetByID(id)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(*user)})
}

// List — GET /api/v1/users
func (h *UserHandler) List(c *gin.Context) {
	const op = "UserHandler.List"

	users, err := h.userRepo.List()
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		out = append(out, toUserResponse(u))
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

// ListTenants — GET /api/v1/users/:id/tenants
// Lista os tenants aos quais o usuário tem acesso (o próprio usuário ou super admin).
func (h *UserHandler) ListTenants(c *gin.Context) {
	const op = "UserHandler.ListTenants"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	tenants, err := h.userTenantRepo.ListTenantsForUser(id)
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

// UpdateEmail — PUT /api/v1/users/:id/email
func (h *UserHandler) UpdateEmail(c *gin.Context) {
	const op = "UserHandler.UpdateEmail"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req updateEmailRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	user := &domain.User{ID: id, Email: req.Email}
	if err := h.userRepo.UpdateEmail(user); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "e-mail atualizado com sucesso"})
}

// UpdatePassword — PUT /api/v1/users/:id/password
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	const op = "UserHandler.UpdatePassword"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	var req updatePasswordRequest
	if err := bindJSON(c, op, &req); err != nil {
		httperr.Respond(c, err)
		return
	}

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		httperr.Respond(c, domain.NewInternal(op, "falha ao gerar hash da senha", err))
		return
	}

	user := &domain.User{ID: id, Password: hashed}
	if err := h.userRepo.UpdatePassword(user); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "senha atualizada com sucesso"})
}

// Activate — PATCH /api/v1/users/:id/activate
func (h *UserHandler) Activate(c *gin.Context) {
	const op = "UserHandler.Activate"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.userRepo.Activate(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "usuário ativado com sucesso"})
}

// Deactivate — PATCH /api/v1/users/:id/deactivate
// Impede novos logins a partir de agora; o token já emitido (se houver) continua
// válido até expirar (máx. 24h) — mesma limitação documentada da ausência de refresh
// token, ver docs/05_Auth_Backend_Contract.md.
func (h *UserHandler) Deactivate(c *gin.Context) {
	const op = "UserHandler.Deactivate"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.userRepo.Deactivate(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "usuário desativado com sucesso"})
}

// Delete — DELETE /api/v1/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	const op = "UserHandler.Delete"

	id, err := userIDParam(c, op)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	if err := h.userRepo.Delete(id); err != nil {
		httperr.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "usuário excluído com sucesso"})
}
