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

type UserService struct {
	repo     *repository.UserRepository
	roleRepo *repository.RoleRepository
	orgRepo  *repository.OrganizationRepository
}

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

func NewUserService(repo *repository.UserRepository, roleRepo *repository.RoleRepository, orgRepo *repository.OrganizationRepository) *UserService {
	return &UserService{repo: repo, roleRepo: roleRepo, orgRepo: orgRepo}
}

func (s *UserService) List(keyword, role string, scope DataScope) ([]model.User, error) {
	var orgIDs []uint64
	if !scope.All {
		orgIDs = scope.OrgIDs
	}
	return s.repo.List(keyword, role, orgIDs)
}

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

func (s *UserService) Delete(id, currentUserID uint64) error {
	if id == currentUserID {
		return errors.New("不能删除当前登录账号")
	}
	return s.repo.Delete(id)
}

type UserImportResult struct {
	Created         int `json:"created"`
	Updated         int `json:"updated"`
	Skipped         int `json:"skipped"`
	DefaultPassword int `json:"default_password"`
}

var userImportHeaders = []string{"姓名", "用户名", "角色编码", "组织编码", "手机", "邮箱", "企业微信UserID", "微信OpenID", "状态", "初始密码"}

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
