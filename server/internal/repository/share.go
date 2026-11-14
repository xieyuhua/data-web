package repository

import (
	"strings"
	"time"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type ShareRepository struct{ db *gorm.DB }

func NewShareRepository(db *gorm.DB) *ShareRepository { return &ShareRepository{db: db} }

func (r *ShareRepository) Create(item *model.ShareLink) error { return r.db.Create(item).Error }

func (r *ShareRepository) FindByHash(hash string) (*model.ShareLink, error) {
	var item model.ShareLink
	if err := r.db.Where("token_hash = ?", hash).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ShareRepository) ListByUser(userID uint64, isAdmin bool) ([]model.ShareLink, error) {
	query := r.db.Model(&model.ShareLink{})
	if !isAdmin {
		query = query.Where("created_by = ?", userID)
	}
	var items []model.ShareLink
	err := query.Order("id desc").Limit(200).Find(&items).Error
	return items, err
}

func (r *ShareRepository) Revoke(id, userID uint64, isAdmin bool) error {
	query := r.db.Model(&model.ShareLink{}).Where("id = ?", id)
	if !isAdmin {
		query = query.Where("created_by = ?", userID)
	}
	return query.Update("revoked", true).Error
}

func (r *ShareRepository) IncrementViews(id uint64) error {
	return r.db.Model(&model.ShareLink{}).Where("id = ?", id).Updates(map[string]any{
		"views":          gorm.Expr("views + 1"),
		"last_viewed_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error
}

func (r *ShareRepository) DB() *gorm.DB { return r.db }

type AuditRepository struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) *AuditRepository { return &AuditRepository{db: db} }

func (r *AuditRepository) Create(item *model.AuditLog) error { return r.db.Create(item).Error }

// AuditFilter 审计日志查询条件。
type AuditFilter struct {
	Keyword  string
	Action   string
	Status   int
	StartAt  *time.Time
	EndAt    *time.Time
	Page     int
	PageSize int
}

// AuditStats 审计概览指标。
type AuditStats struct {
	Total    int64 `json:"total"`
	Today    int64 `json:"today"`
	Failures int64 `json:"failures"`
	Users    int64 `json:"users"`
}

func (r *AuditRepository) Search(filter AuditFilter) ([]model.AuditLog, int64, error) {
	query := r.db.Model(&model.AuditLog{})
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR path LIKE ? OR resource LIKE ?", like, like, like)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Status > 0 {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.StartAt != nil {
		query = query.Where("created_at >= ?", *filter.StartAt)
	}
	if filter.EndAt != nil {
		query = query.Where("created_at < ?", *filter.EndAt)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	size := filter.PageSize
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	var items []model.AuditLog
	err := query.Order("created_at desc, id desc").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AuditRepository) Stats() (AuditStats, error) {
	var stats AuditStats
	if err := r.db.Model(&model.AuditLog{}).Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := r.db.Model(&model.AuditLog{}).Where("created_at >= ?", today).Count(&stats.Today).Error; err != nil {
		return stats, err
	}
	if err := r.db.Model(&model.AuditLog{}).Where("status >= 400").Count(&stats.Failures).Error; err != nil {
		return stats, err
	}
	if err := r.db.Model(&model.AuditLog{}).Where("username <> ''").Distinct("username").Count(&stats.Users).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *AuditRepository) Actions() ([]string, error) {
	var actions []string
	err := r.db.Model(&model.AuditLog{}).Distinct().Order("action asc").Pluck("action", &actions).Error
	return actions, err
}
