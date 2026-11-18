package service

import (
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
	"biaoge-bi/internal/web"
)

// AuditService records security-relevant mutations. Failures are logged, never
// surfaced to the caller.
type AuditService struct {
	repo *repository.Repo
}

func NewAuditService(repo *repository.Repo) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Log(c *gin.Context, action, resourceType string, resourceID uint, detail any) {
	if s == nil || s.repo == nil || c == nil {
		return
	}
	entry := model.AuditLog{
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		IP: c.ClientIP(),
	}
	if u := web.CurrentUser(c); u != nil {
		entry.UserID = u.ID
	}
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			entry.DetailJSON = string(b)
		}
	}
	if err := s.repo.DB.Create(&entry).Error; err != nil {
		slog.Warn("写入审计日志失败", "error", err, "action", action)
	}
}

// List returns recent audit entries (admin only; enforced by the router).
func (s *AuditService) List(page, pageSize int, action string) ([]model.AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	q := s.repo.DB.Model(&model.AuditLog{})
	if action != "" {
		q = q.Where("action = ?", action)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.AuditLog
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}
