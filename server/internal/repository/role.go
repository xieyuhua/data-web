package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// RoleRepository 角色数据访问层。
// 角色用于描述平台内置的层级身份（如管理员、大区经理、门店店长等）。
type RoleRepository struct{ db *gorm.DB }

// NewRoleRepository 创建角色仓储实例。
func NewRoleRepository(db *gorm.DB) *RoleRepository { return &RoleRepository{db: db} }

// FindByCode 按角色编码查询角色（编码唯一）。
func (r *RoleRepository) FindByCode(code string) (*model.Role, error) {
	var role model.Role
	if err := r.db.Where("code = ?", code).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// All 查询全部角色，按层级、ID 升序排列（层级越小权限越高）。
func (r *RoleRepository) All() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Order("level asc, id asc").Find(&roles).Error
	return roles, err
}

// Create 新增角色。
func (r *RoleRepository) Create(role *model.Role) error { return r.db.Create(role).Error }
