package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type RoleRepository struct{ db *gorm.DB }

func NewRoleRepository(db *gorm.DB) *RoleRepository { return &RoleRepository{db: db} }

func (r *RoleRepository) FindByCode(code string) (*model.Role, error) {
	var role model.Role
	if err := r.db.Where("code = ?", code).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepository) All() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Order("level asc, id asc").Find(&roles).Error
	return roles, err
}

func (r *RoleRepository) Create(role *model.Role) error { return r.db.Create(role).Error }
