package service

import (
	"encoding/json"
	"fmt"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"
)

// DataScope 数据权限范围。
// All=true 表示不受限（管理员/总部）；否则只看 OrgIDs 内的组织。
type DataScope struct {
	All    bool
	OrgIDs []uint64
}

// ScopeService 数据权限计算服务。
type ScopeService struct {
	orgRepo  *repository.OrganizationRepository
	roleRepo *repository.RoleRepository
}

// NewScopeService 创建数据权限服务。
func NewScopeService(orgRepo *repository.OrganizationRepository, roleRepo *repository.RoleRepository) *ScopeService {
	return &ScopeService{orgRepo: orgRepo, roleRepo: roleRepo}
}

// Resolve 按角色 data_scope 计算用户可见组织：
// all（全部）、own_org（本组织）、own_org_tree（本组织及下级，默认）。
func (s *ScopeService) Resolve(user *model.User) (DataScope, error) {
	role, err := s.roleRepo.FindByCode(user.RoleCode)
	if err != nil {
		return DataScope{}, fmt.Errorf("用户角色不存在")
	}
	switch role.DataScope {
	case "all":
		ids, err := s.orgRepo.AllIDs()
		return DataScope{All: true, OrgIDs: ids}, err
	case "own_org":
		return DataScope{OrgIDs: []uint64{user.OrganizationID}}, nil
	case "own_org_tree":
		ids, err := s.orgRepo.DescendantIDs(user.OrganizationID)
		return DataScope{OrgIDs: ids}, err
	default:
		return DataScope{OrgIDs: []uint64{user.OrganizationID}}, nil
	}
}

// ResolveForUser 按用户 ID 计算数据权限。
func (s *ScopeService) ResolveForUser(userID uint64, userRepo *repository.UserRepository) (DataScope, error) {
	user, err := userRepo.FindByID(userID)
	if err != nil {
		return DataScope{}, err
	}
	return s.Resolve(user)
}

// AllOrgIDs 返回全部组织 ID，供看板“全量数据”模式与分享范围计算使用。
func (s *ScopeService) AllOrgIDs() ([]uint64, error) {
	return s.orgRepo.AllIDs()
}

// CanAccessOrg 判断用户是否有权访问指定组织。
func (s *ScopeService) CanAccessOrg(user *model.User, orgID uint64) (bool, error) {
	scope, err := s.Resolve(user)
	if err != nil {
		return false, err
	}
	for _, id := range scope.OrgIDs {
		if id == orgID {
			return true, nil
		}
	}
	return false, nil
}

// Permissions 解析角色权限 JSON，返回权限点列表。
func Permissions(role *model.Role) []string {
	if role == nil {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(role.Permissions), &result); err != nil {
		return []string{}
	}
	return result
}

// HasPermission 判断角色是否拥有指定权限，* 表示超级权限。
func HasPermission(role *model.Role, permission string) bool {
	if role == nil {
		return false
	}
	for _, item := range Permissions(role) {
		if item == "*" || item == permission {
			return true
		}
	}
	return false
}

// IsAdmin 判断是否为平台管理员（super_admin 或拥有 * 权限）。
func IsAdmin(role *model.Role) bool {
	return role != nil && (role.Code == "super_admin" || HasPermission(role, "*"))
}
