package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/config"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
	"biaoge-bi/internal/wecom"
)

type WeComService struct {
	repo    *repository.Repo
	cfg     *config.Config
	client  *wecom.Client
	authSvc *AuthService
	runtime *config.Runtime
}

func NewWeComService(repo *repository.Repo, cfg *config.Config, authSvc *AuthService) *WeComService {
	return &WeComService{
		repo: repo, cfg: cfg, client: wecom.NewClient(cfg.WeCom), authSvc: authSvc,
	}
}

// SetRuntime lets WeCom settings change without a restart.
func (s *WeComService) SetRuntime(rt *config.Runtime) { s.runtime = rt }

// liveCfg returns the current config snapshot.
func (s *WeComService) liveCfg() *config.Config {
	if s.runtime != nil {
		return s.runtime.Get()
	}
	return s.cfg
}

// liveClient builds a client from the current settings so credential edits
// take effect immediately.
func (s *WeComService) liveClient() *wecom.Client {
	if s.runtime != nil {
		return wecom.NewClient(s.runtime.Get().WeCom)
	}
	return s.client
}

func (s *WeComService) Enabled() bool { return s.liveClient().Enabled() }

// Status is exposed to the login page so it can render the SSO entry.
type WeComStatus struct {
	Enabled     bool   `json:"enabled"`
	CorpID      string `json:"corpId,omitempty"`
	AgentID     int    `json:"agentId,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
	LastSyncAt  string `json:"lastSyncAt,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"`
	LastMessage string `json:"lastMessage,omitempty"`
}

func (s *WeComService) Status() WeComStatus {
	st := WeComStatus{}
	if !s.Enabled() {
		return st
	}
	st.Enabled = true
	client := s.liveClient()
	st.CorpID = client.CorpID()
	st.AgentID = client.AgentID()
	st.RedirectURI = client.RedirectURI()
	var state model.SyncState
	if err := s.repo.DB.Where("key = ?", "wecom").First(&state).Error; err == nil {
		st.LastSyncAt = state.LastRunAt.Format(time.RFC3339)
		st.LastStatus = state.Status
		st.LastMessage = state.Message
	}
	return st
}

// AuthorizeURL returns the SSO QR URL for the given post-login redirect state.
func (s *WeComService) AuthorizeURL(state string) string {
	return s.liveClient().AuthorizeURL(state, s.cfg.WeCom.RedirectURI)
}

// Login exchanges an OAuth code for a local account, creating it on first use.
func (s *WeComService) Login(ctx context.Context, code string) (*LoginResult, error) {
	if !s.Enabled() {
		return nil, errors.New("企业微信登录未启用")
	}
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("缺少企业微信授权码")
	}
	wecomUserID, err := s.liveClient().UserInfo(ctx, code)
	if err != nil {
		return nil, err
	}
	u, err := s.repo.UserByWeComID(wecomUserID)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		// The user may not have been synced yet; sync on demand then retry.
		if u, err = s.ensureUser(ctx, wecomUserID); err != nil {
			return nil, err
		}
	}
	if !u.Enabled {
		return nil, ErrUserDisabled
	}
	return s.authSvc.issueToken(u)
}

// ensureUser creates or refreshes a local account from the WeCom directory.
func (s *WeComService) ensureUser(ctx context.Context, wecomUserID string) (*model.User, error) {
	if !s.liveCfg().WeCom.AutoCreateUser {
		return nil, errors.New("账号尚未开通，请联系管理员")
	}
	contacts, err := s.liveClient().ListUsers(ctx)
	if err != nil {
		slog.Warn("企业微信通讯录拉取失败，使用最小信息创建账号", "error", err)
		u := model.User{
			Username: wecomUserID, DisplayName: wecomUserID, Role: model.RoleUser,
			Enabled: true, Source: model.SourceWeCom, WeComUserID: wecomUserID,
		}
		if err := s.repo.DB.Create(&u).Error; err != nil {
			return nil, err
		}
		return &u, nil
	}
	for _, c := range contacts {
		if c.UserID != wecomUserID {
			continue
		}
		return s.upsertWeComUser(c, s.adminIDs())
	}
	return nil, errors.New("企业微信通讯录中未找到该成员")
}

func (s *WeComService) adminIDs() map[string]bool {
	return s.liveClient().SuperAdminIDs()
}

// upsertWeComUser creates or updates a local account from a contact record.
func (s *WeComService) upsertWeComUser(c wecom.User, admins map[string]bool) (*model.User, error) {
	deptJSON, _ := json.Marshal(c.Department)
	role := model.RoleUser
	if admins[c.UserID] {
		role = model.RoleAdmin
	}
	enabled := c.Enabled

	u, err := s.repo.UserByWeComID(c.UserID)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		// Prefer an existing local account with the same username, then link it.
		if existing, err2 := s.repo.UserByUsername(c.UserID); err2 == nil {
			existing.WeComUserID = c.UserID
			existing.Source = model.SourceWeCom
			existing.WeComDeptIDs = string(deptJSON)
			existing.Position = c.Position
			existing.Mobile = c.Mobile
			if c.Email != "" {
				existing.Email = c.Email
			}
			if existing.DisplayName == "" {
				existing.DisplayName = c.Name
			}
			if admins[c.UserID] {
				existing.Role = model.RoleAdmin
			}
			if err := s.repo.DB.Save(existing).Error; err != nil {
				return nil, err
			}
			return existing, nil
		}
		// Some secret formats embed an unusable password hash; wecom accounts
		// authenticate exclusively through SSO, so store a random unguessable one.
		randomHash, err := auth.HashPassword(randomPasswordString())
		if err != nil {
			return nil, err
		}
		u = &model.User{
			Username: c.UserID, PasswordHash: randomHash, DisplayName: c.Name,
			Email: c.Email, Role: role, Enabled: enabled, Source: model.SourceWeCom,
			WeComUserID: c.UserID, WeComDeptIDs: string(deptJSON),
			Position: c.Position, Mobile: c.Mobile,
		}
		if err := s.repo.DB.Create(u).Error; err != nil {
			return nil, err
		}
		return u, nil
	}

	u.DisplayName = firstNonEmpty(c.Name, u.DisplayName)
	u.Email = firstNonEmpty(c.Email, u.Email)
	u.WeComDeptIDs = string(deptJSON)
	u.Position = c.Position
	u.Mobile = c.Mobile
	if admins[c.UserID] {
		u.Role = model.RoleAdmin
	}
	if err := s.repo.DB.Save(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func randomPasswordString() string {
	b := make([]byte, 24)
	for i := range b {
		b[i] = byte('a' + (int(time.Now().UnixNano()+int64(i*37)) % 26))
	}
	return string(b)
}

// SyncForTest is a seam used by tests to stub the directory.
var SyncDepartmentsFunc func(ctx context.Context) ([]wecom.Department, error)
var SyncUsersFunc func(ctx context.Context) ([]wecom.User, error)

// SyncResult summarises one directory synchronization run.
type SyncResult struct {
	Departments int       `json:"departments"`
	Users       int       `json:"users"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
}

// Sync pulls departments and members from WeCom into local tables.
func (s *WeComService) Sync(ctx context.Context) (*SyncResult, error) {
	res := &SyncResult{StartedAt: time.Now(), Status: "success"}
	err := s.doSync(ctx, res)
	res.FinishedAt = time.Now()
	if err != nil {
		res.Status = "failed"
		res.Message = err.Error()
	}
	s.recordSyncState(res)
	return res, err
}

func (s *WeComService) doSync(ctx context.Context, res *SyncResult) error {
	if !s.Enabled() {
		return errors.New("企业微信未启用")
	}
	fetchDepts := SyncDepartmentsFunc
	if fetchDepts == nil {
		fetchDepts = s.liveClient().ListDepartments
	}
	fetchUsers := SyncUsersFunc
	if fetchUsers == nil {
		fetchUsers = s.liveClient().ListUsers
	}

	depts, err := fetchDepts(ctx)
	if err != nil {
		return fmt.Errorf("拉取部门失败: %w", err)
	}
	users, err := fetchUsers(ctx)
	if err != nil {
		return fmt.Errorf("拉取成员失败: %w", err)
	}

	admins := s.adminIDs()
	err = s.repo.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, d := range depts {
			var row model.Department
			err := tx.Where("we_com_id = ?", d.ID).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				row = model.Department{WeComID: d.ID}
			} else if err != nil {
				return err
			}
			row.Name = d.Name
			row.ParentWeComID = d.ParentID
			row.SortOrder = d.Order
			row.SyncedAt = now
			if row.ID == 0 {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		for _, c := range users {
			if _, err := s.upsertWeComUserTx(tx, c, admins); err != nil {
				return err
			}
			// Newly disabled accounts lose access immediately.
			if !c.Enabled {
				_ = tx.Model(&model.User{}).Where("we_com_user_id = ?", c.UserID).Update("enabled", false).Error
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	res.Departments = len(depts)
	res.Users = len(users)
	res.Message = "同步完成"
	return nil
}

// upsertWeComUserTx is a transaction-scoped variant of upsertWeComUser.
func (s *WeComService) upsertWeComUserTx(tx *gorm.DB, c wecom.User, admins map[string]bool) (*model.User, error) {
	deptJSON, _ := json.Marshal(c.Department)
	role := model.RoleUser
	if admins[c.UserID] {
		role = model.RoleAdmin
	}
	var u model.User
	err := tx.Where("we_com_user_id = ?", c.UserID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var byName model.User
		if err2 := tx.Where("username = ?", c.UserID).First(&byName).Error; err2 == nil {
			byName.WeComUserID = c.UserID
			byName.Source = model.SourceWeCom
			byName.WeComDeptIDs = string(deptJSON)
			byName.Position = c.Position
			byName.Mobile = c.Mobile
			byName.Email = firstNonEmpty(c.Email, byName.Email)
			byName.DisplayName = firstNonEmpty(c.Name, byName.DisplayName)
			if admins[c.UserID] {
				byName.Role = model.RoleAdmin
			}
			byName.Enabled = c.Enabled
			if err := tx.Save(&byName).Error; err != nil {
				return nil, err
			}
			return &byName, nil
		}
		hash, err := auth.HashPassword(randomPasswordString())
		if err != nil {
			return nil, err
		}
		u = model.User{
			Username: c.UserID, PasswordHash: hash, DisplayName: c.Name, Email: c.Email,
			Role: role, Enabled: c.Enabled, Source: model.SourceWeCom, WeComUserID: c.UserID,
			WeComDeptIDs: string(deptJSON), Position: c.Position, Mobile: c.Mobile,
		}
		if err := tx.Create(&u).Error; err != nil {
			return nil, err
		}
		return &u, nil
	}
	if err != nil {
		return nil, err
	}
	u.DisplayName = firstNonEmpty(c.Name, u.DisplayName)
	u.Email = firstNonEmpty(c.Email, u.Email)
	u.WeComDeptIDs = string(deptJSON)
	u.Position = c.Position
	u.Mobile = c.Mobile
	u.Enabled = c.Enabled
	if admins[c.UserID] {
		u.Role = model.RoleAdmin
	}
	if err := tx.Save(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *WeComService) recordSyncState(res *SyncResult) {
	state := model.SyncState{
		Key: "wecom", LastRunAt: res.FinishedAt, Status: res.Status,
		Message: truncateStr(res.Message, 500), DeptCount: res.Departments, UserCount: res.Users,
	}
	var existing model.SyncState
	if err := s.repo.DB.Where("key = ?", "wecom").First(&existing).Error; err == nil {
		existing.LastRunAt = state.LastRunAt
		existing.Status = state.Status
		existing.Message = state.Message
		existing.DeptCount = state.DeptCount
		existing.UserCount = state.UserCount
		_ = s.repo.DB.Save(&existing).Error
		return
	}
	_ = s.repo.DB.Create(&state).Error
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ---------- Department helpers ----------

// DepartmentView is a department with its tree depth for UI rendering.
type DepartmentView struct {
	WeComID       int    `json:"wecomId"`
	Name          string `json:"name"`
	ParentWeComID int    `json:"parentWecomId"`
	Depth         int    `json:"depth"`
}

// Departments returns the synced department tree ordered parents-first.
func (s *WeComService) Departments() ([]DepartmentView, error) {
	var rows []model.Department
	if err := s.repo.DB.Order("sort_order, we_com_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]model.Department, len(rows))
	for _, r := range rows {
		byID[r.WeComID] = r
	}
	depth := func(d model.Department) int {
		n := 0
		parent := d.ParentWeComID
		for i := 0; i < 20 && parent != 0; i++ {
			p, ok := byID[parent]
			if !ok {
				break
			}
			n++
			parent = p.ParentWeComID
		}
		return n
	}
	// Emit parents before children so clients can build the tree in one pass,
	// and keep sibling order stable by sort_order then id.
	children := make(map[int][]model.Department)
	for _, r := range rows {
		children[r.ParentWeComID] = append(children[r.ParentWeComID], r)
	}
	for parent := range children {
		list := children[parent]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].SortOrder != list[j].SortOrder {
				return list[i].SortOrder < list[j].SortOrder
			}
			return list[i].WeComID < list[j].WeComID
		})
		children[parent] = list
	}

	out := make([]DepartmentView, 0, len(rows))
	visited := make(map[int]bool, len(rows))
	var walk func(parentID int)
	walk = func(parentID int) {
		for _, r := range children[parentID] {
			if visited[r.WeComID] {
				continue
			}
			visited[r.WeComID] = true
			out = append(out, DepartmentView{
				WeComID: r.WeComID, Name: r.Name, ParentWeComID: r.ParentWeComID, Depth: depth(r),
			})
			walk(r.WeComID)
		}
	}
	walk(0)
	// Orphans (parent missing from the table) still need to be visible.
	for _, r := range rows {
		if !visited[r.WeComID] {
			out = append(out, DepartmentView{
				WeComID: r.WeComID, Name: r.Name, ParentWeComID: r.ParentWeComID, Depth: depth(r),
			})
		}
	}
	return out, nil
}

// DirectDeptIDs parses the department list stored on a user record.
func DirectDeptIDs(u *model.User) []int {
	if u == nil || u.WeComDeptIDs == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(u.WeComDeptIDs), &ids); err != nil {
		return nil
	}
	return ids
}

// DescendantAndAncestorDeptIDs returns the given departments plus all nested
// children and all ancestor departments.
func DescendantAndAncestorDeptIDs(db *gorm.DB, roots []int) []int {
	seen := map[int]bool{}
	frontier := append([]int(nil), roots...)
	for len(frontier) > 0 {
		var rows []model.Department
		if err := db.Where("we_com_id IN ?", frontier).Find(&rows).Error; err != nil {
			break
		}
		var next []int
		for _, r := range rows {
			if !seen[r.WeComID] {
				seen[r.WeComID] = true
			}
			if r.ParentWeComID != 0 && !seen[r.ParentWeComID] {
				next = append(next, r.ParentWeComID)
			}
		}
		frontier = next
	}
	for _, id := range roots {
		seen[id] = true
	}
	// Descendants via breadth-first walk.
	down := append([]int(nil), roots...)
	for len(down) > 0 {
		var children []model.Department
		if err := db.Where("parent_we_com_id IN ?", down).Find(&children).Error; err != nil {
			break
		}
		down = down[:0]
		for _, c := range children {
			if !seen[c.WeComID] {
				seen[c.WeComID] = true
				down = append(down, c.WeComID)
			}
		}
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// DescendantDeptIDs expands department ids with all nested children.
func DescendantDeptIDs(db *gorm.DB, roots []int) []int {
	if len(roots) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(roots))
	for _, id := range roots {
		seen[id] = true
	}
	frontier := append([]int(nil), roots...)
	for len(frontier) > 0 {
		var children []model.Department
		if err := db.Where("parent_we_com_id IN ?", frontier).Find(&children).Error; err != nil {
			break
		}
		frontier = frontier[:0]
		for _, c := range children {
			if !seen[c.WeComID] {
				seen[c.WeComID] = true
				frontier = append(frontier, c.WeComID)
			}
		}
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}
