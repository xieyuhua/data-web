package repository

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// ShareRepository 看板分享链接数据访问层。
// 数据库只保存 Token 的哈希值，明文 Token 仅在生成时返回一次。
type ShareRepository struct{ db *gorm.DB }

// NewShareRepository 创建分享仓储实例。
func NewShareRepository(db *gorm.DB) *ShareRepository { return &ShareRepository{db: db} }

// Create 新增分享链接记录。
func (r *ShareRepository) Create(item *model.ShareLink) error { return r.db.Create(item).Error }

// FindByHash 按 Token 哈希查询分享链接。
func (r *ShareRepository) FindByHash(hash string) (*model.ShareLink, error) {
	var item model.ShareLink
	if err := r.db.Where("token_hash = ?", hash).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ListByUser 查询分享链接列表；非管理员只能看到自己创建的链接。
func (r *ShareRepository) ListByUser(userID uint64, isAdmin bool) ([]model.ShareLink, error) {
	query := r.db.Model(&model.ShareLink{})
	if !isAdmin {
		query = query.Where("created_by = ?", userID)
	}
	var items []model.ShareLink
	err := query.Order("id desc").Limit(200).Find(&items).Error
	return items, err
}

// Revoke 撤销分享链接；非管理员只能撤销自己的链接。
func (r *ShareRepository) Revoke(id, userID uint64, isAdmin bool) error {
	query := r.db.Model(&model.ShareLink{}).Where("id = ?", id)
	if !isAdmin {
		query = query.Where("created_by = ?", userID)
	}
	return query.Update("revoked", true).Error
}

// IncrementViews 累加访问次数并刷新最近访问时间。
func (r *ShareRepository) IncrementViews(id uint64) error {
	return r.db.Model(&model.ShareLink{}).Where("id = ?", id).Updates(map[string]any{
		"views":          gorm.Expr("views + 1"),
		"last_viewed_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error
}

// DB 暴露底层连接。
func (r *ShareRepository) DB() *gorm.DB { return r.db }

// AuditRepository 审计日志数据访问层。
type AuditRepository struct{ db *gorm.DB }

// NewAuditRepository 创建审计日志仓储实例。
func NewAuditRepository(db *gorm.DB) *AuditRepository { return &AuditRepository{db: db} }

// Create 写入审计日志。
func (r *AuditRepository) Create(item *model.AuditLog) error { return r.db.Create(item).Error }

// auditShareTokenPattern 匹配分享访问路径中的 Token，用于脱敏。
var auditShareTokenPattern = regexp.MustCompile(`(/public/dashboards/)[^/?#]+`)

// redactAuditPath 将路径中的分享 Token 替换为 {token}，避免明文落库。
func redactAuditPath(path string) string {
	return auditShareTokenPattern.ReplaceAllString(path, "${1}{token}")
}

// Backfill 将旧版日志 Detail 中的 method / path / status 回填到独立字段，
// 同时把历史记录中的分享 Token 脱敏。启动时执行，可重复调用。
// Backfill 将旧版日志 Detail 中的 method / path / status 回填到独立字段，
// 同时把历史记录中的分享 Token 脱敏。启动时执行，可重复调用。
func (r *AuditRepository) Backfill() error {
	var items []model.AuditLog
	err := r.db.
		Where("(method = '' OR method IS NULL) AND detail <> ''").
		Limit(5000).
		Find(&items).Error
	if err != nil {
		return err
	}
	for index := range items {
		item := items[index]
		var detail struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Query  string `json:"query"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal([]byte(item.Detail), &detail); err != nil {
			continue
		}
		path := redactAuditPath(detail.Path)
		detail.Path = path
		action := item.Action
		if action == "POST" && strings.HasSuffix(path, "/auth/login") {
			action = "LOGIN"
		}
		payload, _ := json.Marshal(detail)
		err := r.db.Model(&model.AuditLog{}).Where("id = ?", item.ID).Updates(map[string]any{
			"method": detail.Method,
			"path":   path,
			"status": detail.Status,
			"action": action,
			"detail": string(payload),
		}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// AuditFilter 审计日志查询条件。
// AuditFilter 审计日志查询条件。
type AuditFilter struct {
	Keyword  string
	Action   string
	Status   string
	StartAt  *time.Time
	EndAt    *time.Time
	Page     int
	PageSize int
}

// AuditStats 审计概览指标。
// AuditStats 审计概览指标。
type AuditStats struct {
	Total    int64 `json:"total"`
	Today    int64 `json:"today"`
	Failures int64 `json:"failures"`
	Users    int64 `json:"users"`
}

// Search 分页查询审计日志，支持关键字、动作、状态码与时间范围过滤。
func (r *AuditRepository) Search(filter AuditFilter) ([]model.AuditLog, int64, error) {
	query := r.db.Model(&model.AuditLog{})
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR path LIKE ? OR resource LIKE ?", like, like, like)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		// 支持 success/failed/server 语义值，也支持直接传具体状态码
		switch status {
		case "success":
			query = query.Where("status >= 200 AND status < 300")
		case "failed":
			query = query.Where("status >= 400")
		case "server":
			query = query.Where("status >= 500")
		default:
			if code, err := strconv.Atoi(status); err == nil {
				query = query.Where("status = ?", code)
			}
		}
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

// Stats 统计审计日志概览：总数、今日、失败数、涉及用户数。
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

// Actions 查询日志中出现过的动作类型，供前端下拉筛选。
func (r *AuditRepository) Actions() ([]string, error) {
	var actions []string
	err := r.db.Model(&model.AuditLog{}).Distinct().Order("action asc").Pluck("action", &actions).Error
	return actions, err
}
