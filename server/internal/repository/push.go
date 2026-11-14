package repository

import (
	"bi-platform/internal/model"

	"gorm.io/gorm"
)

type PushRepository struct{ db *gorm.DB }

func NewPushRepository(db *gorm.DB) *PushRepository { return &PushRepository{db: db} }

func (r *PushRepository) ListTemplates() ([]model.PushTemplate, error) {
	var items []model.PushTemplate
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

func (r *PushRepository) FindTemplate(id uint64) (*model.PushTemplate, error) {
	var item model.PushTemplate
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PushRepository) CreateTemplate(item *model.PushTemplate) error {
	return r.db.Create(item).Error
}
func (r *PushRepository) UpdateTemplate(item *model.PushTemplate) error { return r.db.Save(item).Error }
func (r *PushRepository) DeleteTemplate(id uint64) error {
	return r.db.Delete(&model.PushTemplate{}, id).Error
}

func (r *PushRepository) ListSubscriptions() ([]model.Subscription, error) {
	var items []model.Subscription
	err := r.db.Order("id desc").Find(&items).Error
	return items, err
}

func (r *PushRepository) FindSubscription(id uint64) (*model.Subscription, error) {
	var item model.Subscription
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PushRepository) CreateSubscription(item *model.Subscription) error {
	return r.db.Create(item).Error
}
func (r *PushRepository) UpdateSubscription(item *model.Subscription) error {
	return r.db.Save(item).Error
}
func (r *PushRepository) DeleteSubscription(id uint64) error {
	return r.db.Delete(&model.Subscription{}, id).Error
}
func (r *PushRepository) CreateRecord(item *model.PushRecord) error { return r.db.Create(item).Error }

func (r *PushRepository) FindRecord(id uint64) (*model.PushRecord, error) {
	var item model.PushRecord
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PushRepository) ListRecords(limit int) ([]model.PushRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var items []model.PushRecord
	err := r.db.Order("sent_at desc").Limit(limit).Find(&items).Error
	return items, err
}

func (r *PushRepository) ListChannelConfigs() ([]model.ChannelConfig, error) {
	var items []model.ChannelConfig
	err := r.db.Order("id asc").Find(&items).Error
	return items, err
}

func (r *PushRepository) FindChannelConfig(channel string) (*model.ChannelConfig, error) {
	var item model.ChannelConfig
	if err := r.db.Where("channel = ?", channel).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PushRepository) UpsertChannelConfig(item *model.ChannelConfig) error {
	var existing model.ChannelConfig
	err := r.db.Where("channel = ?", item.Channel).First(&existing).Error
	if err == nil {
		item.ID = existing.ID
		return r.db.Save(item).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return r.db.Create(item).Error
}

func (r *PushRepository) DB() *gorm.DB { return r.db }
