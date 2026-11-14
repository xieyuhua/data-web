package repository

import (
	"strings"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// UserRepository 用户数据访问层。
type UserRepository struct{ db *gorm.DB }

// NewUserRepository 创建用户仓储实例。
func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }

// FindByUsername 按用户名查询用户（登录使用）。
func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

// FindByWecomUserID 按企业微信 UserID 查找账号，用于企业微信授权登录。
// FindByWecomUserID 按企业微信 UserID 查询账号，用于企业微信授权登录。
func (r *UserRepository) FindByWecomUserID(wecomUserID string) (*model.User, error) {
	var user model.User
	err := r.db.Where("wecom_user_id = ?", wecomUserID).First(&user).Error
	return &user, err
}

// FindByID 按主键查询用户，并预加载所属组织。
func (r *UserRepository) FindByID(id uint64) (*model.User, error) {
	var user model.User
	err := r.db.Preload("Organization").First(&user, id).Error
	return &user, err
}

// List 按关键字、角色、组织范围查询用户；orgIDs 为 nil 表示不限组织。
func (r *UserRepository) List(keyword, roleCode string, orgIDs []uint64) ([]model.User, error) {
	query := r.db.Model(&model.User{}).Preload("Organization")
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR username LIKE ? OR phone LIKE ?", like, like, like)
	}
	if roleCode != "" {
		query = query.Where("role_code = ?", roleCode)
	}
	if orgIDs != nil {
		if len(orgIDs) == 0 {
			return []model.User{}, nil
		}
		query = query.Where("organization_id IN ?", orgIDs)
	}
	var users []model.User
	err := query.Order("id asc").Find(&users).Error
	return users, err
}

// Create 新增用户。
func (r *UserRepository) Create(user *model.User) error { return r.db.Create(user).Error }

// Update 保存用户。
func (r *UserRepository) Update(user *model.User) error { return r.db.Save(user).Error }

// Delete 按主键删除用户。
func (r *UserRepository) Delete(id uint64) error {
	return r.db.Delete(&model.User{}, id).Error
}

// UpdateLastLogin 更新最近登录时间。
func (r *UserRepository) UpdateLastLogin(id uint64) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("last_login_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

// FindByIDs 按主键批量查询在职用户。
func (r *UserRepository) FindByIDs(ids []uint64) ([]model.User, error) {
	if len(ids) == 0 {
		return []model.User{}, nil
	}
	var users []model.User
	err := r.db.Where("id IN ? AND status = ?", ids, "active").Find(&users).Error
	return users, err
}

// FindRecipients 查询推送接收人：按角色与组织范围筛选在职用户。
func (r *UserRepository) FindRecipients(roles []string, orgIDs []uint64) ([]model.User, error) {
	query := r.db.Where("status = ?", "active")
	if len(roles) > 0 {
		query = query.Where("role_code IN ?", roles)
	}
	if orgIDs != nil {
		if len(orgIDs) == 0 {
			return []model.User{}, nil
		}
		query = query.Where("organization_id IN ?", orgIDs)
	}
	var users []model.User
	err := query.Order("id asc").Find(&users).Error
	return users, err
}

// DB 暴露底层连接。
func (r *UserRepository) DB() *gorm.DB { return r.db }
