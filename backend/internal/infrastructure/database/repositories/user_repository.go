package repositories

import (
	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"
	"errors"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(id uint) (*domain.User, error) {
	const op = "userRepository.GetByID"

	if id == 0 {
		return nil, domain.NewInternal(op, "id is required", nil)
	}

	var model models.User

	err := r.db.First(&model, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "user not found", err)
	}

	if err != nil {
		return nil, domain.NewInternal(op, "failed to get user by id", err)
	}

	return toDomainUser(&model), nil
}

func (r *userRepository) GetByEmail(email string) (*domain.User, error) {
	const op = "userRepository.GetByEmail"

	if email == "" {
		return nil, domain.NewInternal(op, "email is required", nil)
	}

	var model models.User
	err := r.db.First(&model, "email = ?", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "user not found", err)
	}

	if err != nil {
		return nil, domain.NewInternal(op, "failed to get user by email", err)
	}

	return toDomainUser(&model), nil
}

func (r *userRepository) GetByName(name string) (*domain.User, error) {
	const op = "userRepository.GetByName"

	var model models.User

	err := r.db.First(&model, "name = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.NewNotFound(op, "user not found", err)
	}

	if err != nil {
		return nil, domain.NewInternal(op, "failed to get user by name", err)
	}

	return toDomainUser(&model), nil
}

func (r *userRepository) List() ([]domain.User, error) {
	const op = "userRepository.List"
	var modelList []models.User
	err := r.db.Order("id").Find(&modelList).Error
	if err != nil {
		return nil, domain.NewInternal(op, "failed to list users", err)
	}

	return mapUsers(modelList), nil
}

func (r *userRepository) Save(user *domain.User) error {
	const op = "userRepository.Save"

	if user.Name == "" || user.Email == "" || user.Password == "" {
		return domain.NewInternal(op, "name, email, and password are required", nil)
	}

	isEmailAvailable, err := r.CheckEmailAvailability(user.Email)
	if err != nil {
		return domain.NewInternal(op, "failed to check email availability", err)
	}
	if !isEmailAvailable {
		return domain.NewInternal(op, "email is already in use", nil)
	}

	model := toModelUser(user)

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return domain.NewInternal(op, "failed to create user", err)
		}
		return nil
	})

	if err != nil {
		return domain.NewInternal(op, "failed to save user", err)
	}

	user.ID = model.ID
	return nil
}

func (r *userRepository) Delete(id uint) error {
	const op = "userRepository.Delete"

	if id == 0 {
		return domain.NewInternal(op, "id is required", nil)
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.User{}, id).Error; err != nil {
			return domain.NewInternal(op, "failed to delete user", err)
		}
		return nil
	})

	if err != nil {
		return domain.NewInternal(op, "failed to delete user", err)
	}

	return nil
}

func (r *userRepository) UpdatePassword(user *domain.User) error {
	const op = "userRepository.UpdatePassword"

	if user.ID == 0 {
		return domain.NewInternal(op, "user id is required", nil)
	}

	if user.Password == "" {
		return domain.NewInternal(op, "password is required", nil)
	}

	model := toModelUser(user)

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", model.ID).Update("password", model.Password).Error; err != nil {
			return domain.NewInternal(op, "failed to update password", err)
		}
		return nil
	})

	if err != nil {
		return domain.NewInternal(op, "failed to update password", err)
	}

	return nil
}

func (r *userRepository) UpdateEmail(user *domain.User) error {
	const op = "userRepository.UpdateEmail"

	if user.ID == 0 {
		return domain.NewInternal(op, "user id is required", nil)
	}

	if user.Email == "" {
		return domain.NewInternal(op, "email is required", nil)
	}

	isEmailAvailable, err := r.CheckEmailAvailability(user.Email)
	if err != nil {
		return domain.NewInternal(op, "failed to check email availability", err)
	}
	if !isEmailAvailable {
		return domain.NewInternal(op, "email is already in use", nil)
	}

	model := toModelUser(user)

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", model.ID).Update("email", model.Email).Error; err != nil {
			return domain.NewInternal(op, "failed to update email", err)
		}
		return nil
	})

	if err != nil {
		return domain.NewInternal(op, "failed to update email", err)
	}

	return nil
}

func (r *userRepository) UpdateName(user *domain.User) error {
	const op = "userRepository.UpdateName"

	if user.Name == "" {
		return domain.NewInternal(op, "name is required", nil)
	}

	model := toModelUser(user)

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", model.ID).Update("name", model.Name).Error; err != nil {
			return domain.NewInternal(op, "failed to update name", err)
		}
		return nil
	})

	if err != nil {
		return domain.NewInternal(op, "failed to update name", err)
	}

	return nil
}

// ---- Conversion Helpers ---- //

func toDomainUser(model *models.User) *domain.User {
	domainUser := &domain.User{
		ID:       model.ID,
		Name:     model.Name,
		Email:    model.Email,
		Password: model.Password,
	}
	return domainUser
}

func mapUsers(modelList []models.User) []domain.User {
	domainList := make([]domain.User, 0, len(modelList))
	for _, model := range modelList {
		domainList = append(domainList, *toDomainUser(&model))
	}
	return domainList
}

func toModelUser(user *domain.User) models.User {
	modelUser := models.User{
		Name:     user.Name,
		Email:    user.Email,
		Password: user.Password,
	}
	return modelUser
}

func (r *userRepository) CheckEmailAvailability(email string) (bool, error) {
	const op = "userRepository.CheckEmailAvailability"

	var count int64
	if err := r.db.Model(&models.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, domain.NewInternal(op, "failed to check email availability", err)
	}

	return count == 0, nil
}
