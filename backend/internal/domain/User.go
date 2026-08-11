package domain

type User struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserRepository interface {
	Save(user *User) error
	UpdateEmail(user *User) error
	UpdatePassword(user *User) error
	Delete(id uint) error
	GetByID(id uint) (*User, error)
	GetByEmail(email string) (*User, error)
	List() ([]User, error)
	GetByName(name string) (*User, error)
	CheckEmailAvailability(email string) (bool, error)
}
