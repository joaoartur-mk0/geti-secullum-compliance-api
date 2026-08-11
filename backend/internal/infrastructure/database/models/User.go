package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Name         string `gorm:"not null" json:"name"`
	Email        string `gorm:"not null;uniqueIndex" json:"email"`
	Password     string `gorm:"not null" json:"password"`
	IsSuperAdmin bool   `gorm:"not null;default:false" json:"is_super_admin"`
}
