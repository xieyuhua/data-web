package service

import (
	"encoding/json"
	"fmt"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"
)

type DataScope struct {
	All    bool
	OrgIDs []uint64
}

type ScopeService struct {
	orgRepo  *repository.OrganizationRepository
	roleRepo *repository.RoleRepository
}

func NewScopeService(orgRepo *repository.OrganizationRepository, roleRepo *repository.RoleRepository) *ScopeService {
	return &ScopeService{orgRepo: orgRepo, roleRepo: roleRepo}
}

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

func IsAdmin(role *model.Role) bool {
	return role != nil && (role.Code == "super_admin" || HasPermission(role, "*"))
}
