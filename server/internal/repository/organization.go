package repository

import (
	"errors"

	"bi-platform/internal/model"

	"gorm.io/gorm"
)

// OrganizationRepository 组织架构数据访问层。
// 组织采用 parent_id 自关联形成树，level 表示层级（大区/区域/小区/门店等）。
type OrganizationRepository struct{ db *gorm.DB }

// NewOrganizationRepository 创建组织仓储实例。
func NewOrganizationRepository(db *gorm.DB) *OrganizationRepository {
	return &OrganizationRepository{db: db}
}

// All 查询全部组织，按层级、排序号、ID 升序。
func (r *OrganizationRepository) All() ([]model.Organization, error) {
	var items []model.Organization
	err := r.db.Order("level asc, sort asc, id asc").Find(&items).Error
	return items, err
}

// Tree 将平铺的组织列表组装成树。
// 父节点不存在（被删除或数据异常）的节点会被当作根节点处理。
func (r *OrganizationRepository) Tree() ([]model.Organization, error) {
	items, err := r.All()
	if err != nil {
		return nil, err
	}
	byID := make(map[uint64]bool, len(items))
	for _, item := range items {
		byID[item.ID] = true
	}
	byParent := make(map[uint64][]model.Organization)
	roots := make([]model.Organization, 0)
	for _, item := range items {
		item.Children = []model.Organization{}
		if item.ParentID == nil || *item.ParentID == 0 || !byID[*item.ParentID] {
			// 无父节点或父节点缺失，视为根节点
			roots = append(roots, item)
			continue
		}
		byParent[*item.ParentID] = append(byParent[*item.ParentID], item)
	}
	var attach func([]model.Organization) []model.Organization
	// 递归挂载子节点
	attach = func(list []model.Organization) []model.Organization {
		for i := range list {
			list[i].Children = attach(byParent[list[i].ID])
		}
		return list
	}
	return attach(roots), nil
}

// FindByID 按主键查询组织。
func (r *OrganizationRepository) FindByID(id uint64) (*model.Organization, error) {
	var item model.Organization
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// FindByCode 按编码查询组织（编码唯一）。
func (r *OrganizationRepository) FindByCode(code string) (*model.Organization, error) {
	var item model.Organization
	if err := r.db.Where("code = ?", code).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Create 新增组织。
func (r *OrganizationRepository) Create(item *model.Organization) error {
	return r.db.Create(item).Error
}

// Update 保存组织。
func (r *OrganizationRepository) Update(item *model.Organization) error {
	return r.db.Save(item).Error
}

// Delete 删除组织。
// 存在下级组织或组织下仍有用户时拒绝删除，避免出现孤儿数据。
func (r *OrganizationRepository) Delete(id uint64) error {
	var children int64
	if err := r.db.Model(&model.Organization{}).Where("parent_id = ?", id).Count(&children).Error; err != nil {
		return err
	}
	if children > 0 {
		return errors.New("请先删除下级组织")
	}
	var users int64
	if err := r.db.Model(&model.User{}).Where("organization_id = ?", id).Count(&users).Error; err != nil {
		return err
	}
	if users > 0 {
		return errors.New("该组织下仍有用户，无法删除")
	}
	return r.db.Delete(&model.Organization{}, id).Error
}

// DescendantIDs 查询指定组织及其全部下级组织 ID（广度优先）。
func (r *OrganizationRepository) DescendantIDs(rootID uint64) ([]uint64, error) {
	items, err := r.All()
	if err != nil {
		return nil, err
	}
	children := make(map[uint64][]uint64)
	for _, item := range items {
		if item.ParentID != nil {
			children[*item.ParentID] = append(children[*item.ParentID], item.ID)
		}
	}
	result := make([]uint64, 0)
	queue := []uint64{rootID}
	visited := make(map[uint64]bool)
	// 使用队列做广度优先遍历，visited 防止环形数据导致死循环
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		result = append(result, current)
		queue = append(queue, children[current]...)
	}
	return result, nil
}

// AllIDs 查询全部组织 ID。
func (r *OrganizationRepository) AllIDs() ([]uint64, error) {
	items, err := r.All()
	if err != nil {
		return nil, err
	}
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result, nil
}

// Count 统计组织数量。
func (r *OrganizationRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.Organization{}).Count(&count).Error
	return count, err
}

// Upsert 按编码写入或更新组织，供通讯录导入、企业微信同步使用。
func (r *OrganizationRepository) Upsert(item *model.Organization) error {
	var existing model.Organization
	err := r.db.Where("code = ?", item.Code).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(item).Error
	}
	if err != nil {
		return err
	}
	item.ID = existing.ID
	item.CreatedAt = existing.CreatedAt
	return r.db.Save(item).Error
}

// DB 暴露底层连接。
func (r *OrganizationRepository) DB() *gorm.DB { return r.db }
