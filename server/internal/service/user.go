package service

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService 用户（通讯录）业务服务。
type UserService struct {
	repo     *repository.UserRepository
	roleRepo *repository.RoleRepository
	orgRepo  *repository.OrganizationRepository
}

// UserInput 用户新建 / 更新入参。
type UserInput struct {
	Name           string `json:"name"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	RoleCode       string `json:"role_code"`
	OrganizationID uint64 `json:"organization_id"`
	WecomUserID    string `json:"wecom_user_id"`
	WechatOpenID   string `json:"wechat_open_id"`
	Status         string `json:"status"`
}

// NewUserService 创建用户服务。
func NewUserService(repo *repository.UserRepository, roleRepo *repository.RoleRepository, orgRepo *repository.OrganizationRepository) *UserService {
	return &UserService{repo: repo, roleRepo: roleRepo, orgRepo: orgRepo}
}

// List 按数据范围查询用户。
// orgID 大于 0 时只返回该组织下的成员；includeChildren 为 true 时包含全部下级组织。
// 最终组织范围始终与当前账号的数据权限求交集，防止通过前端参数越权查看通讯录。
func (s *UserService) List(keyword, role string, scope DataScope, orgID uint64, includeChildren bool) ([]model.User, error) {
	var orgIDs []uint64 // nil 表示不限组织
	if !scope.All {
		orgIDs = scope.OrgIDs
	}
	if orgID > 0 {
		// 选中部门：默认只查自身，勾选“包含下级”则展开整棵子树
		targets := []uint64{orgID}
		if includeChildren {
			ids, err := s.orgRepo.DescendantIDs(orgID)
			if err != nil {
				return nil, err
			}
			targets = ids
		}
		if orgIDs == nil {
			orgIDs = targets
		} else {
			orgIDs = intersectIDs(orgIDs, targets)
		}
	}
	return s.repo.List(keyword, role, orgIDs)
}

// intersectIDs 求两个 ID 集合的交集，保持 base 的相对顺序。
func intersectIDs(base, other []uint64) []uint64 {
	if len(base) == 0 || len(other) == 0 {
		return []uint64{}
	}
	allow := make(map[uint64]bool, len(other))
	for _, id := range other {
		allow[id] = true
	}
	result := make([]uint64, 0, len(base))
	for _, id := range base {
		if allow[id] {
			result = append(result, id)
		}
	}
	return result
}

// OrgMemberStat 组织成员统计：直属人数、含下级总人数、直属启用人数。
type OrgMemberStat struct {
	OrgID  uint64 `json:"org_id"`
	Direct int64  `json:"direct"`
	Total  int64  `json:"total"`
	Active int64  `json:"active"`
}

// MemberStats 统计各组织人数，用于组织树的人数角标；只返回当前数据范围内的组织。
func (s *UserService) MemberStats(scope DataScope) ([]OrgMemberStat, error) {
	orgs, err := s.orgRepo.All()
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.CountByOrg()
	if err != nil {
		return nil, err
	}

	// 直属人数与直属启用人数
	direct := make(map[uint64]int64, len(rows))
	active := make(map[uint64]int64, len(rows))
	for _, row := range rows {
		direct[row.OrgID] = row.Total
		active[row.OrgID] = row.Active
	}

	parentOf := make(map[uint64]uint64, len(orgs))
	for _, org := range orgs {
		if org.ParentID != nil && *org.ParentID > 0 {
			parentOf[org.ID] = *org.ParentID
		}
	}

	// 把直属人数逐级累加到祖先节点，得到“含下级”人数
	total := make(map[uint64]int64, len(orgs))
	for orgID, count := range direct {
		current := orgID
		for i := 0; i < 32; i++ { // 层级上限保护，避免脏数据形成环导致死循环
			total[current] += count
			parent, ok := parentOf[current]
			if !ok || parent == 0 {
				break
			}
			current = parent
		}
	}

	allowed := make(map[uint64]bool, len(scope.OrgIDs))
	if !scope.All {
		for _, id := range scope.OrgIDs {
			allowed[id] = true
		}
	}
	stats := make([]OrgMemberStat, 0, len(orgs))
	for _, org := range orgs {
		if !scope.All && !allowed[org.ID] {
			continue
		}
		stats = append(stats, OrgMemberStat{
			OrgID:  org.ID,
			Direct: direct[org.ID],
			Total:  total[org.ID],
			Active: active[org.ID],
		})
	}
	return stats, nil
}

// Create 新增用户（密码使用 bcrypt 加密）。
func (s *UserService) Create(input UserInput) (*model.User, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Username) == "" {
		return nil, errors.New("姓名和用户名不能为空")
	}
	if _, err := s.roleRepo.FindByCode(input.RoleCode); err != nil {
		return nil, errors.New("角色不存在")
	}
	if _, err := s.orgRepo.FindByID(input.OrganizationID); err != nil {
		return nil, errors.New("所属组织不存在")
	}
	if _, err := s.repo.FindByUsername(input.Username); err == nil {
		return nil, errors.New("用户名已存在")
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if status == "" {
		status = "active"
	}
	user := &model.User{
		Name: input.Name, Username: strings.TrimSpace(input.Username), PasswordHash: hash,
		Phone: input.Phone, Email: input.Email, RoleCode: input.RoleCode,
		OrganizationID: input.OrganizationID, WecomUserID: input.WecomUserID,
		WechatOpenID: input.WechatOpenID, Status: status,
	}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	return s.repo.FindByID(user.ID)
}

// Update 更新用户。
func (s *UserService) Update(id uint64, input UserInput) (*model.User, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	if input.Name != "" {
		user.Name = input.Name
	}
	if input.RoleCode != "" {
		if _, err := s.roleRepo.FindByCode(input.RoleCode); err != nil {
			return nil, errors.New("角色不存在")
		}
		user.RoleCode = input.RoleCode
	}
	if input.OrganizationID > 0 {
		if _, err := s.orgRepo.FindByID(input.OrganizationID); err != nil {
			return nil, errors.New("所属组织不存在")
		}
		user.OrganizationID = input.OrganizationID
	}
	if input.Password != "" {
		if len(input.Password) < 6 {
			return nil, errors.New("密码至少需要 6 位")
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = string(hashed)
	}
	user.Phone = input.Phone
	user.Email = input.Email
	user.WecomUserID = input.WecomUserID
	user.WechatOpenID = input.WechatOpenID
	if input.Status != "" {
		user.Status = input.Status
	}
	if err := s.repo.Update(user); err != nil {
		return nil, err
	}
	return s.repo.FindByID(user.ID)
}

// Delete 删除用户（禁止删除当前登录账号）。
func (s *UserService) Delete(id, currentUserID uint64) error {
	if id == currentUserID {
		return errors.New("不能删除当前登录账号")
	}
	return s.repo.Delete(id)
}

// UserImportResult 通讯录导入结果统计。
type UserImportResult struct {
	Created         int `json:"created"`
	Updated         int `json:"updated"`
	Skipped         int `json:"skipped"`
	DefaultPassword int `json:"default_password"`
}

var userImportHeaders = []string{"姓名", "用户名", "角色编码", "组织编码", "手机", "邮箱", "企业微信UserID", "微信OpenID", "状态", "初始密码"}

// Export 导出通讯录 Excel。
func (s *UserService) Export() ([]byte, error) {
	users, err := s.repo.List("", "", nil)
	if err != nil {
		return nil, err
	}
	orgs, err := s.orgRepo.All()
	if err != nil {
		return nil, err
	}
	orgMap := make(map[uint64]model.Organization, len(orgs))
	for _, item := range orgs {
		orgMap[item.ID] = item
	}
	file := excelize.NewFile()
	defer file.Close()
	sheet := "通讯录"
	index, _ := file.NewSheet(sheet)
	file.SetActiveSheet(index)
	_ = file.DeleteSheet("Sheet1")
	writeHeader(file, sheet, userImportHeaders, "173A33")
	for rowIndex, user := range users {
		orgCode := ""
		if org, ok := orgMap[user.OrganizationID]; ok {
			orgCode = org.Code
		}
		values := []any{
			user.Name, user.Username, user.RoleCode, orgCode, user.Phone, user.Email,
			user.WecomUserID, user.WechatOpenID, user.Status, "",
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowIndex+2)
			_ = file.SetCellValue(sheet, cell, value)
		}
	}
	_ = file.SetColWidth(sheet, "A", "J", 18)
	_ = file.SetColWidth(sheet, "B", "B", 20)
	_ = file.SetColWidth(sheet, "G", "H", 24)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Template 生成通讯录导入模板。
func (s *UserService) Template() ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	sheet := "通讯录"
	index, _ := file.NewSheet(sheet)
	file.SetActiveSheet(index)
	_ = file.DeleteSheet("Sheet1")
	writeHeader(file, sheet, userImportHeaders, "173A33")
	sample := []any{
		"张三", "zhangsan", "store_manager", "ST_SH_JA", "13800000000",
		"zhangsan@example.com", "zhangsan", "zhangsan_openid", "active", "123456",
	}
	for col, value := range sample {
		cell, _ := excelize.CoordinatesToCellName(col+1, 2)
		_ = file.SetCellValue(sheet, cell, value)
	}
	_ = file.SetColWidth(sheet, "A", "J", 18)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Import 通讯录 Excel，按用户名幂等更新；新用户初始密码为空时使用 123456。
func (s *UserService) Import(filename string, reader io.Reader) (*UserImportResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".xlsx" && ext != ".xls" {
		return nil, errors.New("请上传 Excel 文件（.xlsx）")
	}
	file, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, fmt.Errorf("无法读取 Excel: %w", err)
	}
	defer file.Close()
	rows := pickSheetRows(file, "姓名", "用户名")
	if len(rows) < 2 {
		return nil, errors.New("Excel 中没有可导入的数据，请确认表头包含“姓名”和“用户名”")
	}
	headers := map[string]int{}
	for i, title := range rows[0] {
		headers[strings.TrimSpace(title)] = i
	}
	get := func(row []string, name string) string {
		index, ok := headers[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	roles, err := s.roleRepo.All()
	if err != nil {
		return nil, err
	}
	roleByCode := make(map[string]model.Role, len(roles))
	roleByName := make(map[string]model.Role, len(roles))
	for _, item := range roles {
		roleByCode[item.Code] = item
		roleByName[item.Name] = item
	}
	orgs, err := s.orgRepo.All()
	if err != nil {
		return nil, err
	}
	orgByCode := make(map[string]model.Organization, len(orgs))
	orgByName := make(map[string]model.Organization, len(orgs))
	for _, item := range orgs {
		orgByCode[item.Code] = item
		orgByName[item.Name] = item
	}
	result := &UserImportResult{}
	for _, row := range rows[1:] {
		name := get(row, "姓名")
		username := get(row, "用户名")
		if name == "" && username == "" {
			continue
		}
		if name == "" || username == "" {
			return result, fmt.Errorf("用户名 %s 缺少姓名或用户名", username)
		}
		role, ok := roleByCode[get(row, "角色编码")]
		if !ok {
			role, ok = roleByName[get(row, "角色编码")]
		}
		if !ok {
			return result, fmt.Errorf("用户 %s 的角色不存在: %s", username, get(row, "角色编码"))
		}
		org, ok := orgByCode[get(row, "组织编码")]
		if !ok {
			org, ok = orgByName[get(row, "组织编码")]
		}
		if !ok {
			return result, fmt.Errorf("用户 %s 的组织不存在: %s", username, get(row, "组织编码"))
		}
		status := firstNonEmpty(get(row, "状态"), "active")
		password := get(row, "初始密码")
		existing, findErr := s.repo.FindByUsername(username)
		if findErr == nil {
			existing.Name = name
			existing.RoleCode = role.Code
			existing.OrganizationID = org.ID
			existing.Phone = get(row, "手机")
			existing.Email = get(row, "邮箱")
			existing.WecomUserID = get(row, "企业微信UserID")
			existing.WechatOpenID = get(row, "微信OpenID")
			existing.Status = status
			if password != "" {
				if len(password) < 6 {
					return result, fmt.Errorf("用户 %s 的初始密码至少需要 6 位", username)
				}
				hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if hashErr != nil {
					return result, hashErr
				}
				existing.PasswordHash = string(hash)
			}
			if err := s.repo.Update(existing); err != nil {
				return result, err
			}
			result.Updated++
			continue
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return result, findErr
		}
		if password == "" {
			password = "123456"
			result.DefaultPassword++
		}
		if len(password) < 6 {
			return result, fmt.Errorf("用户 %s 的初始密码至少需要 6 位", username)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return result, err
		}
		user := &model.User{
			Name: name, Username: username, PasswordHash: string(hash), Phone: get(row, "手机"),
			Email: get(row, "邮箱"), RoleCode: role.Code, OrganizationID: org.ID,
			WecomUserID: get(row, "企业微信UserID"), WechatOpenID: get(row, "微信OpenID"), Status: status,
		}
		if err := s.repo.Create(user); err != nil {
			return result, err
		}
		result.Created++
	}
	return result, nil
}
