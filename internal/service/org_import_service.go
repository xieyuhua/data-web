package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"biaoge-bi/internal/export"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

// OrgImportService exports and imports the local organization snapshot for
// reconciliation. WeCom stays the source of truth; imported rows are merged by
// username/userid so manual corrections survive until the next sync.
type OrgImportService struct {
	repo *repository.Repo
}

func NewOrgImportService(repo *repository.Repo) *OrgImportService {
	return &OrgImportService{repo: repo}
}

var orgHeader = []string{
	"用户名", "姓名", "企业微信ID", "部门编码(逗号分隔)", "职位", "邮箱", "手机", "角色(admin/user)", "状态(在职/停用)", "来源(wecom/local)",
}

// ExportOrg renders the current users and departments to an xlsx workbook.
func (s *OrgImportService) ExportOrg() ([]byte, error) {
	var users []model.User
	if err := s.repo.DB.Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	rows := make([]export.Cell, 0, len(users))
	for _, u := range users {
		role := "user"
		if u.Role == model.RoleAdmin {
			role = "admin"
		}
		status := "在职"
		if !u.Enabled {
			status = "停用"
		}
		source := u.Source
		if source == "" {
			source = model.SourceLocal
		}
		rows = append(rows, export.Cell{
			u.Username, u.DisplayName, u.WeComUserID, u.WeComDeptIDs, u.Position,
			u.Email, u.Mobile, role, status, source,
		})
	}

	var depts []model.Department
	_ = s.repo.DB.Order("we_com_id").Find(&depts).Error
	deptRows := make([]export.Cell, 0, len(depts))
	for _, d := range depts {
		deptRows = append(deptRows, export.Cell{d.WeComID, d.Name, d.ParentWeComID, d.SortOrder})
	}

	return export.WriteMultiSheet([]export.SheetSpec{
		{Name: "成员", Header: orgHeader, Rows: rows},
		{Name: "部门", Header: []string{"部门ID", "部门名称", "上级部门ID", "排序"}, Rows: deptRows},
	})
}

// RowIssue describes a rejected or adjusted row for the reconciliation report.
type RowIssue struct {
	Row    int    `json:"row"`
	Key    string `json:"key"`
	Level  string `json:"level"` // error|warning
	Reason string `json:"reason"`
}

// OrgImportResult summarizes one import run.
type OrgImportResult struct {
	Total     int        `json:"total"`
	Created   int        `json:"created"`
	Updated   int        `json:"updated"`
	Skipped   int        `json:"skipped"`
	Failed    int        `json:"failed"`
	Issues    []RowIssue `json:"issues"`
	ReportXLSX []byte    `json:"-"`
}

func colIndex(header []string, names ...string) int {
	for i, h := range header {
		clean := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(h), "*"))
		for _, n := range names {
			if clean == n {
				return i
			}
		}
	}
	return -1
}

func cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// ImportOrg parses an exported workbook and upserts users by username.
func (s *OrgImportService) ImportOrg(data []byte) (*OrgImportResult, error) {
	rows, err := export.ParseWorkbook(data)
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("文件中没有数据行")
	}
	header := rows[0]
	iUser := colIndex(header, "用户名", "账号", "username")
	iName := colIndex(header, "姓名", "displayName")
	iWeCom := colIndex(header, "企业微信ID", "企微ID", "wecomUserId")
	iDept := colIndex(header, "部门编码(逗号分隔)", "部门编码", "wecomDeptIds")
	iPos := colIndex(header, "职位", "position")
	iMail := colIndex(header, "邮箱", "email")
	iMobile := colIndex(header, "手机", "mobile")
	iRole := colIndex(header, "角色(admin/user)", "角色", "role")
	iStatus := colIndex(header, "状态(在职/停用)", "状态", "status")
	if iUser < 0 {
		return nil, errors.New("缺少「用户名」列")
	}

	res := &OrgImportResult{}
	deptNames := map[string]bool{}
	var depts []model.Department
	_ = s.repo.DB.Find(&depts).Error
	for _, d := range depts {
		deptNames[strconv.Itoa(d.WeComID)] = true
	}

	for r := 1; r < len(rows); r++ {
		row := rows[r]
		username := cell(row, iUser)
		if username == "" {
			continue
		}
		res.Total++
		rowNo := r + 1

		// Validate departments so typos surface in the report instead of
		// silently detaching people from the org tree.
		deptRaw := cell(row, iDept)
		if deptRaw != "" && deptRaw != "[]" {
			ids := parseDeptList(deptRaw)
			var unknown []string
			for _, id := range ids {
				if !deptNames[id] {
					unknown = append(unknown, id)
				}
			}
			if len(unknown) > 0 {
				res.Issues = append(res.Issues, RowIssue{
					Row: rowNo, Key: username, Level: "warning",
					Reason: "未知部门ID: " + strings.Join(unknown, ","),
				})
			}
		}

		role := model.RoleUser
		switch strings.ToLower(cell(row, iRole)) {
		case "admin", "管理员":
			role = model.RoleAdmin
		}
		enabled := true
		switch strings.ToLower(cell(row, iStatus)) {
		case "停用", "disabled", "false", "0":
			enabled = false
		}

		var existing model.User
		err := s.repo.DB.Where("username = ?", username).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New accounts need a password; SSO accounts get an unusable hash.
			hash, herr := authHashRandom()
			if herr != nil {
				res.Failed++
				res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: username, Level: "error", Reason: "创建账号失败"})
				continue
			}
			u := model.User{
				Username: username, PasswordHash: hash,
				DisplayName: cell(row, iName), Email: cell(row, iMail),
				Mobile: cell(row, iMobile), Position: cell(row, iPos),
				Role: role, Enabled: enabled, Source: model.SourceLocal,
				WeComUserID: cell(row, iWeCom), WeComDeptIDs: deptRaw,
			}
			if u.DisplayName == "" {
				u.DisplayName = username
			}
			if err := s.repo.DB.Create(&u).Error; err != nil {
				res.Failed++
				res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: username, Level: "error", Reason: err.Error()})
				continue
			}
			res.Created++
			continue
		}
		if err != nil {
			res.Failed++
			res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: username, Level: "error", Reason: err.Error()})
			continue
		}

		// Local accounts keep their password and role unless the sheet says
		// otherwise; WeCom-synced accounts are skipped for role changes.
		updates := map[string]any{
			"display_name":   firstNonEmpty(cell(row, iName), existing.DisplayName),
			"email":          firstNonEmpty(cell(row, iMail), existing.Email),
			"mobile":         firstNonEmpty(cell(row, iMobile), existing.Mobile),
			"position":       firstNonEmpty(cell(row, iPos), existing.Position),
			"enabled":        enabled,
			"we_com_dept_ids": firstNonEmpty(deptRaw, existing.WeComDeptIDs),
		}
		if wc := cell(row, iWeCom); wc != "" && existing.WeComUserID == "" {
			updates["we_com_user_id"] = wc
			updates["source"] = model.SourceWeCom
		}
		if existing.Source != model.SourceWeCom {
			updates["role"] = role
		} else if role == model.RoleAdmin && existing.Role != model.RoleAdmin {
			res.Issues = append(res.Issues, RowIssue{
				Row: rowNo, Key: username, Level: "warning",
				Reason: "企微同步账号的角色由管理员白名单控制，已忽略表格中的角色变更",
			})
		}
		if err := s.repo.DB.Model(&model.User{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			res.Failed++
			res.Issues = append(res.Issues, RowIssue{Row: rowNo, Key: username, Level: "error", Reason: err.Error()})
			continue
		}
		res.Updated++
	}

	report, _ := export.WriteMultiSheet([]export.SheetSpec{
		{
			Name:   "导入结果",
			Header: []string{"总行数", "新增", "更新", "失败", "提示"},
			Rows: []export.Cell{{
				res.Total, res.Created, res.Updated, res.Failed,
				formatIssueSummary(res.Issues),
			}},
		},
		{
			Name:   "问题明细",
			Header: []string{"行号", "账号/编码", "级别", "原因"},
			Rows:   issuesToCells(res.Issues),
		},
	})
	res.ReportXLSX = report
	return res, nil
}

func parseDeptList(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(raw), "[]")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func issuesToCells(issues []RowIssue) []export.Cell {
	rows := make([]export.Cell, 0, len(issues))
	for _, is := range issues {
		level := "错误"
		if is.Level == "warning" {
			level = "提示"
		}
		rows = append(rows, export.Cell{is.Row, is.Key, level, is.Reason})
	}
	return rows
}

func formatIssueSummary(issues []RowIssue) string {
	if len(issues) == 0 {
		return "全部通过"
	}
	errs, warns := 0, 0
	for _, i := range issues {
		if i.Level == "error" {
			errs++
		} else {
			warns++
		}
	}
	return fmt.Sprintf("%d 个错误，%d 个提示，详见「问题明细」", errs, warns)
}
