package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// PushRepository 推送相关数据访问层。
// 覆盖推送模板、订阅计划、推送记录与渠道配置四类数据。
type PushRepository struct{ db *gorm.DB }

// NewPushRepository 创建推送仓储实例。
func NewPushRepository(db *gorm.DB) *PushRepository { return &PushRepository{db: db} }

// ListTemplates 查询全部推送模板。
func (r *PushRepository) ListTemplates() ([]model.PushTemplate, error) {
	var items []model.PushTemplate
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

// FindTemplate 按主键查询推送模板。
func (r *PushRepository) FindTemplate(id uint64) (*model.PushTemplate, error) {
	var item model.PushTemplate
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateTemplate 新增推送模板。
func (r *PushRepository) CreateTemplate(item *model.PushTemplate) error {
	return r.db.Create(item).Error
}

// UpdateTemplate 保存推送模板。
func (r *PushRepository) UpdateTemplate(item *model.PushTemplate) error { return r.db.Save(item).Error }

// DeleteTemplate 按主键删除推送模板。
func (r *PushRepository) DeleteTemplate(id uint64) error {
	return r.db.Delete(&model.PushTemplate{}, id).Error
}

// ListSubscriptions 查询全部推送订阅（按创建时间倒序）。
func (r *PushRepository) ListSubscriptions() ([]model.Subscription, error) {
	var items []model.Subscription
	err := r.db.Order("id desc").Find(&items).Error
	return items, err
}

// FindSubscription 按主键查询推送订阅。
func (r *PushRepository) FindSubscription(id uint64) (*model.Subscription, error) {
	var item model.Subscription
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateSubscription 新增推送订阅。
func (r *PushRepository) CreateSubscription(item *model.Subscription) error {
	return r.db.Create(item).Error
}

// UpdateSubscription 保存推送订阅（含下次执行时间）。
func (r *PushRepository) UpdateSubscription(item *model.Subscription) error {
	return r.db.Save(item).Error
}

// DeleteSubscription 按主键删除推送订阅。
func (r *PushRepository) DeleteSubscription(id uint64) error {
	return r.db.Delete(&model.Subscription{}, id).Error
}

// CreateRecord 写入一条推送记录。
func (r *PushRepository) CreateRecord(item *model.PushRecord) error { return r.db.Create(item).Error }

// FindRecord 按主键查询推送记录。
func (r *PushRepository) FindRecord(id uint64) (*model.PushRecord, error) {
	var item model.PushRecord
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ListRecords 查询最近的推送记录，limit 默认 100、上限 500。
func (r *PushRepository) ListRecords(limit int) ([]model.PushRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var items []model.PushRecord
	err := r.db.Order("sent_at desc").Limit(limit).Find(&items).Error
	return items, err
}

// ListChannelConfigs 查询全部渠道配置（企业微信、微信等）。
func (r *PushRepository) ListChannelConfigs() ([]model.ChannelConfig, error) {
	var items []model.ChannelConfig
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

// FindChannelConfig 按渠道标识查询配置。
func (r *PushRepository) FindChannelConfig(channel string) (*model.ChannelConfig, error) {
	var item model.ChannelConfig
	if err := r.db.Where("channel = ?", channel).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// UpsertChannelConfig 按渠道标识写入或更新渠道配置。
func (r *PushRepository) UpsertChannelConfig(item *model.ChannelConfig) error {
	var existing model.ChannelConfig
	err := r.db.Where("channel = ?", item.Channel).First(&existing).Error
	if err == nil {
		// 已存在则复用原 ID 做更新，保证渠道配置唯一
		item.ID = existing.ID
		return r.db.Save(item).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return r.db.Create(item).Error
}

// DB 暴露底层连接。
func (r *PushRepository) DB() *gorm.DB { return r.db }
