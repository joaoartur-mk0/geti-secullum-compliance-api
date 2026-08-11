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
	// tenant sem precisar de uma segunda chamada logo após o login.
	tenants, err := h.userTenantRepo.ListTenantsForUser(user.ID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	tenantIDs := make([]int, 0, len(tenants))
	for _, t := range tenants {
		tenantIDs = append(tenantIDs, t.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"user":       toUserResponse(*user),
		"tenant_ids": tenantIDs,
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
