package repository

import (
	"strings"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type UserRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *UserRepository) FindByID(id uint64) (*model.User, error) {
	var user model.User
	err := r.db.Preload("Organization").First(&user, id).Error
	return &user, err
}

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

func (r *UserRepository) Create(user *model.User) error { return r.db.Create(user).Error }

func (r *UserRepository) Update(user *model.User) error { return r.db.Save(user).Error }

func (r *UserRepository) Delete(id uint64) error {
	return r.db.Delete(&model.User{}, id).Error
}

func (r *UserRepository) UpdateLastLogin(id uint64) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("last_login_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

func (r *UserRepository) FindByIDs(ids []uint64) ([]model.User, error) {
	if len(ids) == 0 {
		return []model.User{}, nil
	}
	var users []model.User
	err := r.db.Where("id IN ? AND status = ?", ids, "active").Find(&users).Error
	return users, err
}

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

func (r *UserRepository) DB() *gorm.DB { return r.db }
