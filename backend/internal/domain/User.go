package domain

type User struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	Active       bool   `json:"active"`
}

type UserRepository interface {
	Save(user *User) error
	UpdateEmail(user *User) error
	UpdatePassword(user *User) error
	Delete(id uint) error
	Activate(id uint) error
	Deactivate(id uint) error
	// SetSuperAdmin promove/rebaixa um usuário a super admin. A alteração só vale no
	// PRÓXIMO LOGIN do alvo — is_super_admin está dentro do JWT já emitido, e não há
	// revogação de token hoje (docs/08 §7.3).
	SetSuperAdmin(id uint, isSuperAdmin bool) error
	GetByID(id uint) (*User, error)
	GetByEmail(email string) (*User, error)
	List() ([]User, error)
	GetByName(name string) (*User, error)
	CheckEmailAvailability(email string) (bool, error)
}
