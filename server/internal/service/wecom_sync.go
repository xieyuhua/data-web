package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/secretbox"
	"bi-platform/internal/repository"
	"bi-platform/internal/wecom"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	wecomSource        = "wecom"
	defaultCodePrefix  = "WECOM_DEPT_"
	defaultRoleCode    = "store_staff"
	maxJobErrors       = 20
	maxJobErrorTextLen = 4000
)

var (
	// ErrWecomSyncRunning 同一时间只允许一个同步任务。
	ErrWecomSyncRunning = errors.New("已有企业微信同步任务正在执行，请稍后再试")
	orgTypeValues       = map[string]bool{"company": true, "big_region": true, "region": true, "store": true}
)

// WecomTypeRules 部门层级到组织类型的映射规则。
type WecomTypeRules struct {
	DepthTypes []string `json:"depth_types"`
	LastType   string   `json:"last_type"`
}

// defaultTypeRules 默认按「集团 → 大区 → 区域 → 门店」映射企业微信部门层级。
func defaultTypeRules() WecomTypeRules {
	return WecomTypeRules{
		DepthTypes: []string{"company", "big_region", "region", "store"},
		LastType:   "store",
	}
}

// normalizeTypeRules 校验并补全层级映射规则，非法类型会被拒绝。
func normalizeTypeRules(input WecomTypeRules) (WecomTypeRules, error) {
	rules := defaultTypeRules()
	if len(input.DepthTypes) > 0 {
		rules.DepthTypes = make([]string, 0, len(input.DepthTypes))
		for _, item := range input.DepthTypes {
			value := strings.TrimSpace(item)
			if !orgTypeValues[value] {
				return rules, fmt.Errorf("组织类型映射不合法: %s", value)
			}
			rules.DepthTypes = append(rules.DepthTypes, value)
		}
		if len(rules.DepthTypes) > 8 {
			return rules, errors.New("组织类型映射最多支持 8 层")
		}
	}
	if last := strings.TrimSpace(input.LastType); last != "" {
		if !orgTypeValues[last] {
			return rules, fmt.Errorf("末级组织类型不合法: %s", last)
		}
		rules.LastType = last
	}
	if rules.LastType == "" {
		rules.LastType = rules.DepthTypes[len(rules.DepthTypes)-1]
	}
	return rules, nil
}

// typeForDepth 返回指定深度对应的组织类型，超过配置层级时使用末级类型。
func (r WecomTypeRules) typeForDepth(depth int) string {
	rules, err := normalizeTypeRules(r)
	if err != nil {
		rules = defaultTypeRules()
	}
	if depth < 1 {
		depth = 1
	}
	if depth <= len(rules.DepthTypes) {
		return rules.DepthTypes[depth-1]
	}
	return rules.LastType
}

// WecomConfigInput 保存配置的入参；Secret 为空或掩码时保留原值。
type WecomConfigInput struct {
	CorpID          string         `json:"corp_id"`
	AgentID         string         `json:"agent_id"`
	Secret          string         `json:"secret"`
	Enabled         *bool          `json:"enabled"`
	SyncUsers       *bool          `json:"sync_users"`
	LoginEnabled    *bool          `json:"login_enabled"`
	LoginMode       string         `json:"login_mode"`
	AutoLogin       *bool          `json:"auto_login"`
	DefaultRoleCode string         `json:"default_role_code"`
	CodePrefix      string         `json:"code_prefix"`
	TypeRules       WecomTypeRules `json:"type_rules"`
}

// WecomConfigView 对外返回的配置，绝不包含明文 Secret。
type WecomConfigView struct {
	CorpID           string         `json:"corp_id"`
	AgentID          string         `json:"agent_id"`
	SecretConfigured bool           `json:"secret_configured"`
	SecretMasked     string         `json:"secret_masked"`
	Enabled          bool           `json:"enabled"`
	SyncUsers        bool           `json:"sync_users"`
	LoginEnabled     bool           `json:"login_enabled"`
	LoginMode        string         `json:"login_mode"`
	AutoLogin        bool           `json:"auto_login"`
	DefaultRoleCode  string         `json:"default_role_code"`
	CodePrefix       string         `json:"code_prefix"`
	TypeRules        WecomTypeRules `json:"type_rules"`
	UpdatedAt        *time.Time     `json:"updated_at,omitempty"`
}

// WecomTestResult 连接测试结果。
type WecomTestResult struct {
	OK              bool     `json:"ok"`
	Message         string   `json:"message"`
	LatencyMS       int64    `json:"latency_ms"`
	DepartmentCount int      `json:"department_count"`
	UserCount       int      `json:"user_count"`
	RootNames       []string `json:"root_names"`
}

// WecomSyncInput 发起同步的入参，为空时使用已保存配置。
type WecomSyncInput struct {
	SyncUsers       *bool          `json:"sync_users"`
	DefaultRoleCode string         `json:"default_role_code"`
	CodePrefix      string         `json:"code_prefix"`
	TypeRules       WecomTypeRules `json:"type_rules"`
	DisableMissing  *bool          `json:"disable_missing"`
}

type wecomSyncOptions struct {
	SyncUsers       bool
	DefaultRoleCode string
	CodePrefix      string
	Rules           WecomTypeRules
	DisableMissing  bool
}

// WecomSyncService 企业微信通讯录异步同步服务。
type WecomSyncService struct {
	repo *repository.WecomRepository
	box  *secretbox.Box

	newClient func(corpID, secret string) wecom.Client

	mu      sync.Mutex
	running bool
}

// NewWecomSyncService 创建企业微信同步服务。
func NewWecomSyncService(repo *repository.WecomRepository, box *secretbox.Box) *WecomSyncService {
	return &WecomSyncService{
		repo: repo,
		box:  box,
		newClient: func(corpID, secret string) wecom.Client {
			return wecom.NewAPIClient(corpID, secret)
		},
	}
}

// Config 返回脱敏后的配置视图，前端只能拿到 Secret 掩码。
func (s *WecomSyncService) Config() (*WecomConfigView, error) {
	item, err := s.repo.Config()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rules := defaultTypeRules()
			return &WecomConfigView{
				Enabled: false, SyncUsers: true, LoginMode: "auto", AutoLogin: true,
				CodePrefix: defaultCodePrefix, TypeRules: rules,
			}, nil
		}
		return nil, err
	}
	return s.toConfigView(item)
}

// SaveConfig 保存配置：Secret 为空或为掩码时保留原密文，避免误覆盖。
func (s *WecomSyncService) SaveConfig(input WecomConfigInput) (*WecomConfigView, error) {
	item, err := s.repo.Config()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		item = &model.WecomSyncConfig{SyncUsers: true, CodePrefix: defaultCodePrefix}
	}
	corpID := strings.TrimSpace(input.CorpID)
	if corpID == "" {
		return nil, errors.New("请填写企业 CorpID")
	}
	item.CorpID = corpID
	item.AgentID = strings.TrimSpace(input.AgentID)
	if secret := strings.TrimSpace(input.Secret); secret != "" && !isMaskedSecret(secret) {
		encrypted, err := s.box.Encrypt(secret)
		if err != nil {
			return nil, fmt.Errorf("加密企业微信 Secret 失败: %w", err)
		}
		item.SecretEnc = encrypted
	}
	if item.SecretEnc == "" {
		return nil, errors.New("请填写企业微信应用 Secret")
	}
	if input.Enabled != nil {
		item.Enabled = *input.Enabled
	}
	if input.SyncUsers != nil {
		item.SyncUsers = *input.SyncUsers
	}
	if input.LoginEnabled != nil {
		item.LoginEnabled = *input.LoginEnabled
	}
	if input.AutoLogin != nil {
		item.AutoLogin = *input.AutoLogin
	}
	if mode := strings.TrimSpace(input.LoginMode); mode != "" {
		if mode != "auto" && mode != "qr" && mode != "oauth" {
			return nil, errors.New("登录方式仅支持 auto / qr / oauth")
		}
		item.LoginMode = mode
	}
	if item.LoginMode == "" {
		item.LoginMode = "auto"
	}
	item.DefaultRoleCode = strings.TrimSpace(input.DefaultRoleCode)
	prefix := strings.TrimSpace(input.CodePrefix)
	if prefix == "" {
		prefix = defaultCodePrefix
	}
	if len(prefix) > 24 {
		return nil, errors.New("组织编码前缀不能超过 24 个字符")
	}
	item.CodePrefix = prefix
	rules, err := normalizeTypeRules(input.TypeRules)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	item.TypeRules = string(encoded)
	if err := s.repo.SaveConfig(item); err != nil {
		return nil, err
	}
	return s.toConfigView(item)
}

// toConfigView 将数据库模型转换为对外视图，并解析层级映射 JSON。
func (s *WecomSyncService) toConfigView(item *model.WecomSyncConfig) (*WecomConfigView, error) {
	rules := defaultTypeRules()
	if strings.TrimSpace(item.TypeRules) != "" {
		var parsed WecomTypeRules
		if err := json.Unmarshal([]byte(item.TypeRules), &parsed); err == nil {
			if normalized, err := normalizeTypeRules(parsed); err == nil {
				rules = normalized
			}
		}
	}
	prefix := item.CodePrefix
	if prefix == "" {
		prefix = defaultCodePrefix
	}
	updated := item.UpdatedAt
	view := &WecomConfigView{
		CorpID: item.CorpID, AgentID: item.AgentID,
		SecretConfigured: item.SecretEnc != "",
		Enabled:          item.Enabled, SyncUsers: item.SyncUsers,
		LoginEnabled: item.LoginEnabled, LoginMode: firstNonEmpty(item.LoginMode, "auto"), AutoLogin: item.AutoLogin,
		DefaultRoleCode: item.DefaultRoleCode, CodePrefix: prefix,
		TypeRules: rules,
	}
	if !updated.IsZero() {
		view.UpdatedAt = &updated
	}
	if view.SecretConfigured {
		view.SecretMasked = "••••••••"
	}
	return view, nil
}

// Test 校验企业微信配置可用性，返回部门数与成员数概览。
func (s *WecomSyncService) Test(ctx context.Context, input WecomConfigInput) (*WecomTestResult, error) {
	corpID := strings.TrimSpace(input.CorpID)
	secret := strings.TrimSpace(input.Secret)
	if isMaskedSecret(secret) {
		secret = ""
	}
	if item, err := s.repo.Config(); err == nil {
		if corpID == "" {
			corpID = item.CorpID
		}
		if secret == "" {
			secret, err = s.box.Decrypt(item.SecretEnc)
			if err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if corpID == "" {
		return nil, errors.New("请先填写并保存 CorpID")
	}
	if secret == "" {
		return nil, errors.New("请先填写并保存企业微信应用 Secret")
	}
	client := s.newClient(corpID, secret)
	start := time.Now()
	departments, err := client.Departments(ctx)
	if err != nil {
		return &WecomTestResult{OK: false, Message: err.Error(), LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	if len(departments) == 0 {
		return &WecomTestResult{OK: false, Message: "企业微信未返回任何部门，请检查应用通讯录权限", LatencyMS: time.Since(start).Milliseconds()}, nil
	}
	byID := make(map[int64]wecom.Department, len(departments))
	for _, item := range departments {
		byID[item.ID] = item
	}
	roots := make([]wecom.Department, 0)
	for _, item := range departments {
		if _, ok := byID[item.ParentID]; !ok || item.ParentID == 0 || item.ParentID == item.ID {
			roots = append(roots, item)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	userCount := 0
	if len(roots) > 0 {
		if users, userErr := client.Users(ctx, roots[0].ID, true); userErr == nil {
			userCount = len(users)
		}
	}
	names := make([]string, 0, len(roots))
	for _, item := range roots {
		names = append(names, item.Name)
	}
	message := fmt.Sprintf("连接成功，共 %d 个部门", len(departments))
	if userCount > 0 {
		message += fmt.Sprintf("，根部门含 %d 名成员", userCount)
	}
	return &WecomTestResult{
		OK: true, Message: message, LatencyMS: time.Since(start).Milliseconds(),
		DepartmentCount: len(departments), UserCount: userCount, RootNames: names,
	}, nil
}

// Start 创建异步同步任务并立即返回任务快照。
func (s *WecomSyncService) Start(userID uint64, input WecomSyncInput) (*model.WecomSyncJob, error) {
	cfg, err := s.repo.Config()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("请先完成企业微信配置")
		}
		return nil, err
	}
	if !cfg.Enabled {
		return nil, errors.New("企业微信同步未启用，请先在配置中启用")
	}
	secret, err := s.box.Decrypt(cfg.SecretEnc)
	if err != nil {
		return nil, err
	}
	if cfg.CorpID == "" || secret == "" {
		return nil, errors.New("企业微信配置不完整，请检查 CorpID 与 Secret")
	}
	rules, err := normalizeTypeRules(input.TypeRules)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.TypeRules) != "" && len(input.TypeRules.DepthTypes) == 0 {
		var saved WecomTypeRules
		if err := json.Unmarshal([]byte(cfg.TypeRules), &saved); err == nil {
			if normalized, err := normalizeTypeRules(saved); err == nil {
				rules = normalized
			}
		}
	}
	syncUsers := cfg.SyncUsers
	if input.SyncUsers != nil {
		syncUsers = *input.SyncUsers
	}
	roleCode, err := s.resolveRole(firstNonEmpty(input.DefaultRoleCode, cfg.DefaultRoleCode))
	if err != nil {
		return nil, err
	}
	prefix := firstNonEmpty(input.CodePrefix, cfg.CodePrefix, defaultCodePrefix)
	options := wecomSyncOptions{
		SyncUsers: syncUsers, DefaultRoleCode: roleCode, CodePrefix: prefix, Rules: rules,
		DisableMissing: input.DisableMissing != nil && *input.DisableMissing,
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, ErrWecomSyncRunning
	}
	if _, err := s.repo.RunningJob(); err == nil {
		s.mu.Unlock()
		return nil, ErrWecomSyncRunning
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		s.mu.Unlock()
		return nil, err
	}
	job := &model.WecomSyncJob{
		Status: "pending", SyncUsers: syncUsers, CreatedBy: userID,
		Message: "任务已创建，等待执行",
	}
	if err := s.repo.CreateJob(job); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.running = true
	s.mu.Unlock()

	go s.run(job.ID, cfg, secret, options)
	return job, nil
}

func (s *WecomSyncService) resolveRole(code string) (string, error) {
	roles, err := s.repo.Roles()
	if err != nil {
		return "", err
	}
	if len(roles) == 0 {
		return "", errors.New("系统中还没有角色，请先初始化角色")
	}
	byCode := make(map[string]model.Role, len(roles))
	for _, item := range roles {
		byCode[item.Code] = item
	}
	if code = strings.TrimSpace(code); code != "" {
		if _, ok := byCode[code]; !ok {
			return "", fmt.Errorf("默认角色不存在: %s", code)
		}
		return code, nil
	}
	if _, ok := byCode[defaultRoleCode]; ok {
		return defaultRoleCode, nil
	}
	// 兜底使用权限层级最低的角色，避免同步出来的账号获得过高权限。
	return roles[len(roles)-1].Code, nil
}

func (s *WecomSyncService) run(jobID uint64, cfg *model.WecomSyncConfig, secret string, opts wecomSyncOptions) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("企业微信同步任务 %d 异常终止: %v", jobID, r)
			s.finishJob(jobID, "failed", fmt.Sprintf("同步异常终止: %v", r))
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	job, err := s.repo.Job(jobID)
	if err != nil {
		log.Printf("企业微信同步任务 %d 读取失败: %v", jobID, err)
		return
	}
	now := time.Now()
	job.Status = "running"
	job.StartedAt = &now
	job.Message = "正在连接企业微信…"
	_ = s.repo.UpdateJob(job)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	client := s.newClient(cfg.CorpID, secret)

	departments, err := client.Departments(ctx)
	if err != nil {
		s.finishJobWithError(job, err)
		return
	}
	if len(departments) == 0 {
		s.finishJobWithError(job, errors.New("企业微信未返回任何部门"))
		return
	}

	sorted, depths, roots := sortDepartments(departments)
	job.Total = len(sorted)
	job.Departments = len(sorted)
	job.RootName = rootNames(roots)
	job.Message = fmt.Sprintf("已获取 %d 个部门，开始同步…", len(sorted))
	_ = s.repo.UpdateJob(job)

	orgIDs := make(map[int64]uint64, len(sorted))
	levels := make(map[uint64]int, len(sorted))
	seenExternal := make([]string, 0, len(sorted))
	for _, dept := range sorted {
		externalID := strconv.FormatInt(dept.ID, 10)
		org, action, err := s.syncDepartment(dept, depths[dept.ID], orgIDs, levels, opts)
		if err != nil {
			job.Failed++
			appendJobError(job, fmt.Sprintf("部门 %s(%d): %v", dept.Name, dept.ID, err))
		} else {
			orgIDs[dept.ID] = org.ID
			levels[org.ID] = org.Level
			seenExternal = append(seenExternal, externalID)
			switch action {
			case "created":
				job.Created++
			case "updated":
				job.Updated++
			}
		}
		job.Processed++
		job.Message = fmt.Sprintf("正在同步部门：%s", dept.Name)
		_ = s.repo.UpdateJob(job)
	}

	if opts.SyncUsers {
		users := s.fetchUsers(ctx, client, roots, job)
		job.Total = len(sorted) + len(users)
		job.Message = fmt.Sprintf("部门同步完成，开始同步 %d 名成员…", len(users))
		_ = s.repo.UpdateJob(job)
		var defaultHash string
		if len(users) > 0 {
			if hash, hashErr := bcrypt.GenerateFromPassword([]byte(randomPassword(24)), bcrypt.DefaultCost); hashErr == nil {
				defaultHash = string(hash)
			}
		}
		for _, user := range users {
			action, err := s.syncUser(user, orgIDs, opts.DefaultRoleCode, defaultHash)
			if err != nil {
				job.Failed++
				appendJobError(job, fmt.Sprintf("成员 %s: %v", user.Name, err))
			} else {
				switch action {
				case "created":
					job.Created++
				case "updated":
					job.Updated++
				case "skipped":
					job.Skipped++
				}
			}
			job.Processed++
			job.Message = fmt.Sprintf("正在同步成员：%s", firstNonEmpty(user.Name, user.UserID))
			_ = s.repo.UpdateJob(job)
		}
	}

	if opts.DisableMissing && job.Failed == 0 {
		if disabled, err := s.repo.DisableMissingWecomOrgs(seenExternal); err != nil {
			appendJobError(job, fmt.Sprintf("停用已移除部门失败: %v", err))
		} else if disabled > 0 {
			job.Message = fmt.Sprintf("已停用 %d 个企业微信中不存在的部门", disabled)
		}
	}

	status := "success"
	if job.Failed > 0 {
		if job.Created+job.Updated > 0 {
			status = "partial"
		} else {
			status = "failed"
		}
	}
	finished := time.Now()
	job.Status = status
	job.FinishedAt = &finished
	job.Message = fmt.Sprintf(
		"同步完成：部门 %d 个，新增 %d，更新 %d，跳过 %d，失败 %d",
		job.Departments, job.Created, job.Updated, job.Skipped, job.Failed,
	)
	_ = s.repo.UpdateJob(job)
}

// syncDepartment 按 external_id 幂等写入单个部门，返回动作 created / updated。
func (s *WecomSyncService) syncDepartment(dept wecom.Department, depth int, orgIDs map[int64]uint64, levels map[uint64]int, opts wecomSyncOptions) (*model.Organization, string, error) {
	externalID := strconv.FormatInt(dept.ID, 10)
	name := strings.TrimSpace(dept.Name)
	if name == "" {
		name = "部门 " + externalID
	}
	var parentID *uint64
	level := 1
	if mapped, ok := orgIDs[dept.ParentID]; ok && dept.ParentID != dept.ID {
		id := mapped
		parentID = &id
		if parentLevel, ok := levels[mapped]; ok {
			level = parentLevel + 1
		}
	}
	orgType := opts.Rules.typeForDepth(depth)

	org, err := s.repo.FindOrgByExternal(wecomSource, externalID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		org = nil
	}
	if org != nil {
		org.Name = name
		org.ParentID = parentID
		org.Type = orgType
		org.Level = level
		org.Sort = dept.Order
		org.Status = "active"
		if err := s.repo.UpdateOrg(org); err != nil {
			return nil, "", err
		}
		return org, "updated", nil
	}
	code, err := s.allocateCode(opts.CodePrefix, dept.ID)
	if err != nil {
		return nil, "", err
	}
	org = &model.Organization{
		ParentID: parentID, Name: name, Code: code, Type: orgType, Level: level,
		Status: "active", Sort: dept.Order, ExternalSource: wecomSource, ExternalID: externalID,
	}
	if err := s.repo.CreateOrg(org); err != nil {
		return nil, "", err
	}
	return org, "created", nil
}

func (s *WecomSyncService) allocateCode(prefix string, deptID int64) (string, error) {
	base := prefix + strconv.FormatInt(deptID, 10)
	candidate := base
	for i := 1; i <= 50; i++ {
		_, err := s.repo.OrgByCode(candidate)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = base + "_" + strconv.Itoa(i)
	}
	return "", fmt.Errorf("无法生成唯一组织编码: %s", base)
}

// fetchUsers 逐个根部门拉取成员并按 userid 去重。
func (s *WecomSyncService) fetchUsers(ctx context.Context, client wecom.Client, roots []wecom.Department, job *model.WecomSyncJob) []wecom.User {
	seen := make(map[string]bool)
	result := make([]wecom.User, 0)
	for _, root := range roots {
		list, err := client.Users(ctx, root.ID, true)
		if err != nil {
			job.Failed++
			appendJobError(job, fmt.Sprintf("读取部门 %s 成员失败: %v", root.Name, err))
			continue
		}
		for _, user := range list {
			if user.UserID == "" || seen[user.UserID] {
				continue
			}
			seen[user.UserID] = true
			result = append(result, user)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UserID < result[j].UserID })
	return result
}

// syncUser 幂等同步成员：按企微 UserID 匹配，未匹配时尝试用户名，仍不存在则新建。
func (s *WecomSyncService) syncUser(user wecom.User, orgIDs map[int64]uint64, roleCode, defaultHash string) (string, error) {
	if strings.TrimSpace(user.UserID) == "" {
		return "skipped", nil
	}
	if user.Status == 5 {
		// 已离职成员不自动建立账号。
		return "skipped", nil
	}
	targetOrg := uint64(0)
	for _, deptID := range user.Department {
		if id, ok := orgIDs[deptID]; ok {
			targetOrg = id
			break
		}
	}
	if targetOrg == 0 {
		if id, ok := orgIDs[user.MainDepartment]; ok {
			targetOrg = id
		}
	}
	if targetOrg == 0 {
		return "skipped", fmt.Errorf("未找到所属部门（部门 %v 未同步成功）", user.Department)
	}
	status := "active"
	if user.Status == 2 {
		status = "disabled"
	}

	existing, err := s.repo.UserByWecomID(user.UserID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		existing = nil
	}
	if existing == nil {
		// 兼容手工创建时用户名就是企微 UserID 的账号，直接建立绑定关系。
		if matched, findErr := s.repo.UserByUsername(user.UserID); findErr == nil {
			existing = matched
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return "", findErr
		}
	}
	if existing != nil {
		if name := strings.TrimSpace(user.Name); name != "" {
			existing.Name = name
		}
		if user.Mobile != "" {
			existing.Phone = user.Mobile
		}
		if user.Email != "" {
			existing.Email = user.Email
		}
		existing.OrganizationID = targetOrg
		existing.WecomUserID = user.UserID
		if user.Status == 1 || user.Status == 2 {
			existing.Status = status
		}
		if err := s.repo.UpdateUser(existing); err != nil {
			return "", err
		}
		return "updated", nil
	}
	username, err := s.uniqueUsername(user.UserID)
	if err != nil {
		return "", err
	}
	if defaultHash == "" {
		return "", errors.New("初始化账号密码失败")
	}
	created := &model.User{
		Name: firstNonEmpty(strings.TrimSpace(user.Name), user.UserID), Username: username,
		PasswordHash: defaultHash, Phone: user.Mobile, Email: user.Email,
		RoleCode: roleCode, OrganizationID: targetOrg, WecomUserID: user.UserID, Status: status,
	}
	if err := s.repo.CreateUser(created); err != nil {
		return "", err
	}
	return "created", nil
}

func (s *WecomSyncService) uniqueUsername(base string) (string, error) {
	name := strings.TrimSpace(base)
	if name == "" {
		name = "wecom_user"
	}
	if len(name) > 48 {
		name = name[:48]
	}
	candidate := name
	for i := 1; i <= 50; i++ {
		_, err := s.repo.UserByUsername(candidate)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s_%d", name, i)
	}
	return "", fmt.Errorf("无法生成唯一用户名: %s", base)
}

// Jobs 返回最近的企业微信同步任务列表。
func (s *WecomSyncService) Jobs(limit int) ([]model.WecomSyncJob, error) {
	return s.repo.Jobs(limit)
}

// Job 按 ID 返回单个同步任务，供前端轮询进度。
func (s *WecomSyncService) Job(id uint64) (*model.WecomSyncJob, error) {
	item, err := s.repo.Job(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("同步任务不存在")
		}
		return nil, err
	}
	return item, nil
}

// Recover 启动时清理中断任务。
func (s *WecomSyncService) Recover() {
	count, err := s.repo.FailStaleJobs()
	if err != nil {
		log.Printf("企业微信同步任务恢复失败: %v", err)
		return
	}
	if count > 0 {
		log.Printf("已将 %d 个中断的企业微信同步任务标记为失败", count)
	}
}

func (s *WecomSyncService) finishJob(jobID uint64, status, message string) {
	job, err := s.repo.Job(jobID)
	if err != nil {
		return
	}
	finished := time.Now()
	job.Status = status
	job.Message = message
	job.FinishedAt = &finished
	_ = s.repo.UpdateJob(job)
}

func (s *WecomSyncService) finishJobWithError(job *model.WecomSyncJob, err error) {
	finished := time.Now()
	job.Status = "failed"
	job.Message = "同步失败：" + err.Error()
	job.FinishedAt = &finished
	appendJobError(job, err.Error())
	_ = s.repo.UpdateJob(job)
}

func appendJobError(job *model.WecomSyncJob, message string) {
	lines := 0
	if strings.TrimSpace(job.Error) != "" {
		lines = strings.Count(job.Error, "\n") + 1
	}
	if lines >= maxJobErrors {
		return
	}
	next := job.Error
	if next != "" {
		next += "\n"
	}
	next += message
	if len([]rune(next)) > maxJobErrorTextLen {
		return
	}
	job.Error = next
}

func sortDepartments(items []wecom.Department) ([]wecom.Department, map[int64]int, []wecom.Department) {
	byID := make(map[int64]wecom.Department, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	depths := make(map[int64]int, len(items))
	var walk func(id int64, seen map[int64]bool) int
	walk = func(id int64, seen map[int64]bool) int {
		if depth, ok := depths[id]; ok {
			return depth
		}
		if seen[id] {
			return 1
		}
		seen[id] = true
		depth := 1
		item := byID[id]
		if parent, ok := byID[item.ParentID]; ok && parent.ID != id {
			depth = walk(parent.ID, seen) + 1
		}
		depths[id] = depth
		return depth
	}
	roots := make([]wecom.Department, 0)
	for _, item := range items {
		_ = walk(item.ID, map[int64]bool{})
		if _, ok := byID[item.ParentID]; !ok || item.ParentID == 0 || item.ParentID == item.ID {
			roots = append(roots, item)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].Order != roots[j].Order {
			return roots[i].Order < roots[j].Order
		}
		return roots[i].ID < roots[j].ID
	})
	sorted := make([]wecom.Department, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		di, dj := depths[sorted[i].ID], depths[sorted[j].ID]
		if di != dj {
			return di < dj
		}
		if sorted[i].Order != sorted[j].Order {
			return sorted[i].Order < sorted[j].Order
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted, depths, roots
}

func rootNames(roots []wecom.Department) string {
	names := make([]string, 0, len(roots))
	for _, item := range roots {
		names = append(names, item.Name)
	}
	return strings.Join(names, "、")
}

func isMaskedSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '•' && r != '*' {
			return false
		}
	}
	return true
}

func randomPassword(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, length)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			buf[i] = alphabet[i%len(alphabet)]
			continue
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf)
}
