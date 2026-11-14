package repository

import (
	"time"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// WecomRepository 企业微信同步相关的持久化操作。
type WecomRepository struct{ db *gorm.DB }

// NewWecomRepository 创建企业微信仓储实例。
func NewWecomRepository(db *gorm.DB) *WecomRepository { return &WecomRepository{db: db} }

// Config 读取企业微信同步配置（系统内仅保留一条配置）。
func (r *WecomRepository) Config() (*model.WecomSyncConfig, error) {
	var item model.WecomSyncConfig
	if err := r.db.Order("id asc").First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// SaveConfig 保存同步配置；Secret 已在服务层加密，这里只做持久化。
func (r *WecomRepository) SaveConfig(item *model.WecomSyncConfig) error {
	return r.db.Save(item).Error
}

// CreateJob 创建一条异步同步任务记录。
func (r *WecomRepository) CreateJob(job *model.WecomSyncJob) error {
	return r.db.Create(job).Error
}

// UpdateJob 保存任务进度；同步协程会高频调用以刷新 processed/total 等字段。
func (r *WecomRepository) UpdateJob(job *model.WecomSyncJob) error {
	return r.db.Save(job).Error
}

// Job 按主键查询单个同步任务。
func (r *WecomRepository) Job(id uint64) (*model.WecomSyncJob, error) {
	var item model.WecomSyncJob
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Jobs 按创建时间倒序返回最近的同步任务，limit 默认 20、最大 100。
func (r *WecomRepository) Jobs(limit int) ([]model.WecomSyncJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []model.WecomSyncJob
	err := r.db.Order("id desc").Limit(limit).Find(&items).Error
	return items, err
}

// RunningJob 查询是否存在 pending / running 状态的任务，用于并发互斥。
func (r *WecomRepository) RunningJob() (*model.WecomSyncJob, error) {
	var item model.WecomSyncJob
	err := r.db.Where("status IN ?", []string{"pending", "running"}).Order("id desc").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FailStaleJobs 服务重启时把残留的进行中任务标记为失败，避免进度永远卡住。
func (r *WecomRepository) FailStaleJobs() (int64, error) {
	now := time.Now()
	result := r.db.Model(&model.WecomSyncJob{}).
		Where("status IN ?", []string{"pending", "running"}).
		Updates(map[string]any{
			"status":      "failed",
			"message":     "服务重启，同步任务已中断，请重新发起",
			"error":       "服务重启导致任务中断",
			"finished_at": &now,
		})
	return result.RowsAffected, result.Error
}

// FindOrgByExternal 按外部来源 + 外部 ID 查找组织，实现企业微信部门幂等同步。
func (r *WecomRepository) FindOrgByExternal(source, externalID string) (*model.Organization, error) {
	var item model.Organization
	err := r.db.Where("external_source = ? AND external_id = ?", source, externalID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// OrgByCode 按组织编码查询，用于生成唯一的企业微信临时编码。
func (r *WecomRepository) OrgByCode(code string) (*model.Organization, error) {
	var item model.Organization
	err := r.db.Where("code = ?", code).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// OrgByID 按主键查询组织。
func (r *WecomRepository) OrgByID(id uint64) (*model.Organization, error) {
	var item model.Organization
	err := r.db.First(&item, id).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateOrg 新建组织节点。
func (r *WecomRepository) CreateOrg(item *model.Organization) error { return r.db.Create(item).Error }

// UpdateOrg 更新组织节点（含名称、父级、层级、类型与状态）。
func (r *WecomRepository) UpdateOrg(item *model.Organization) error { return r.db.Save(item).Error }

// DisableMissingWecomOrgs 把已从企业微信移除的组织标记为停用（仅在本次同步全部成功时调用）。
func (r *WecomRepository) DisableMissingWecomOrgs(seen []string) (int64, error) {
	if len(seen) == 0 {
		return 0, nil
	}
	result := r.db.Model(&model.Organization{}).
		Where("external_source = ? AND external_id <> '' AND external_id NOT IN ? AND status = ?", "wecom", seen, "active").
		Update("status", "disabled")
	return result.RowsAffected, result.Error
}

// UserByWecomID 按企业微信 UserID 查找已绑定账号。
func (r *WecomRepository) UserByWecomID(wecomUserID string) (*model.User, error) {
	var item model.User
	err := r.db.Where("wecom_user_id = ?", wecomUserID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UserByUsername 按用户名查找账号，用于兼容“用户名即企微 UserID”的历史数据。
func (r *WecomRepository) UserByUsername(username string) (*model.User, error) {
	var item model.User
	err := r.db.Where("username = ?", username).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateUser 创建同步生成的成员账号。
func (r *WecomRepository) CreateUser(item *model.User) error { return r.db.Create(item).Error }

// UpdateUser 更新成员资料、所属组织与启停状态。
func (r *WecomRepository) UpdateUser(item *model.User) error { return r.db.Save(item).Error }

// RoleByCode 按角色编码查询角色。
func (r *WecomRepository) RoleByCode(code string) (*model.Role, error) {
	var item model.Role
	err := r.db.Where("code = ?", code).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Roles 按权限层级升序返回全部角色，供新成员选择默认角色。
func (r *WecomRepository) Roles() ([]model.Role, error) {
	var items []model.Role
	err := r.db.Order("level asc, id asc").Find(&items).Error
	return items, err
}

// DB 暴露底层连接，便于测试或后续扩展事务。
func (r *WecomRepository) DB() *gorm.DB { return r.db }
