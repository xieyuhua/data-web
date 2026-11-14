package service

import "bi-platform/internal/model"

// DashboardModelFromView 将看板视图对象还原为模型，便于统一做权限判断。
func DashboardModelFromView(view *DashboardView) *model.Dashboard {
	if view == nil {
		return nil
	}
	return &model.Dashboard{
		ID:             view.ID,
		Status:         view.Status,
		CreatedBy:      view.CreatedBy,
		VisibilityMode: view.VisibilityMode,
		VisibleRoles:   marshalJSON(view.VisibleRoles),
		VisibleUsers:   marshalJSON(view.VisibleUserIDs),
		VisibleOrgs:    marshalJSON(view.VisibleOrgIDs),
		ScopeMode:      view.ScopeMode,
		DrillEnabled:   view.DrillEnabled,
		DrillMode:      view.DrillMode,
	}
}

// CanViewDashboard 统一判断用户是否有权查看某个看板。
// 规则优先级：管理员 > 创建者 > 未发布仅创建者/管理员可见 > 可见范围配置。
func CanViewDashboard(user *model.User, role *model.Role, dashboard *model.Dashboard, orgIDs []uint64) bool {
	if dashboard == nil {
		return false
	}
	if IsAdmin(role) {
		return true
	}
	if user != nil && dashboard.CreatedBy == user.ID {
		return true
	}
	if dashboard.Status != "published" {
		return false
	}
	switch normalizeVisibilityMode(dashboard.VisibilityMode) {
	case "roles":
		return dashboardMatchRole(dashboard, user)
	case "users":
		return dashboardMatchUser(dashboard, user)
	case "orgs":
		return dashboardMatchOrg(dashboard, orgIDs)
	case "custom":
		roles := parseStringList(dashboard.VisibleRoles)
		users := parseUintList(dashboard.VisibleUsers)
		orgs := parseUintList(dashboard.VisibleOrgs)
		if len(roles) == 0 && len(users) == 0 && len(orgs) == 0 {
			return true
		}
		return dashboardMatchRole(dashboard, user) || dashboardMatchUser(dashboard, user) || dashboardMatchOrg(dashboard, orgIDs)
	default:
		return true
	}
}

// DashboardQueryOrgIDs 计算看板查询时使用的组织范围。
// scope_mode=all 时返回全量组织，否则沿用登录人的角色数据范围。
func DashboardQueryOrgIDs(dashboard *model.Dashboard, scope DataScope, allOrgIDs []uint64) []uint64 {
	if dashboard != nil && dashboard.ScopeMode == "all" && len(allOrgIDs) > 0 {
		return allOrgIDs
	}
	return scope.OrgIDs
}

func dashboardMatchRole(dashboard *model.Dashboard, user *model.User) bool {
	if user == nil {
		return false
	}
	for _, code := range parseStringList(dashboard.VisibleRoles) {
		if code == user.RoleCode {
			return true
		}
	}
	return false
}

func dashboardMatchUser(dashboard *model.Dashboard, user *model.User) bool {
	if user == nil {
		return false
	}
	for _, id := range parseUintList(dashboard.VisibleUsers) {
		if id == user.ID {
			return true
		}
	}
	return false
}

func dashboardMatchOrg(dashboard *model.Dashboard, orgIDs []uint64) bool {
	visible := parseUintList(dashboard.VisibleOrgs)
	if len(visible) == 0 || len(orgIDs) == 0 {
		return false
	}
	allowed := make(map[uint64]bool, len(orgIDs))
	for _, id := range orgIDs {
		allowed[id] = true
	}
	for _, id := range visible {
		if allowed[id] {
			return true
		}
	}
	return false
}
