package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"bi-platform/internal/channel"
	"bi-platform/internal/model"
	"bi-platform/internal/repository"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type RecipientRule struct {
	UserIDs []uint64 `json:"user_ids"`
	Roles   []string `json:"roles"`
	OrgIDs  []uint64 `json:"org_ids"`
}

type SubscriptionInput struct {
	Name        string        `json:"name"`
	DashboardID uint64        `json:"dashboard_id"`
	Cron        string        `json:"cron"`
	Timezone    string        `json:"timezone"`
	Channels    []string      `json:"channels"`
	Recipients  RecipientRule `json:"recipients"`
	TemplateID  *uint64       `json:"template_id"`
	Enabled     bool          `json:"enabled"`
}

type SubscriptionView struct {
	model.Subscription
	Channels      []string      `json:"channels"`
	Recipients    RecipientRule `json:"recipients"`
	DashboardName string        `json:"dashboard_name"`
	TemplateName  string        `json:"template_name"`
}

type PushTemplateInput struct {
	Name            string `json:"name"`
	Channel         string `json:"channel"`
	TitleTemplate   string `json:"title_template"`
	ContentTemplate string `json:"content_template"`
	StyleJSON       string `json:"style_json"`
	Enabled         bool   `json:"enabled"`
}

type ChannelConfigView struct {
	Channel   string         `json:"channel"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type PushPreview struct {
	Title   string         `json:"title"`
	Content string         `json:"content"`
	URL     string         `json:"url"`
	Style   map[string]any `json:"style"`
}

type SendSummary struct {
	Total     int    `json:"total"`
	Sent      int    `json:"sent"`
	Simulated int    `json:"simulated"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	Status    string `json:"status"`
}

type PushService struct {
	repo     *repository.PushRepository
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
	orgRepo  *repository.OrganizationRepository
	dashRepo *repository.DashboardRepository
	scope    *ScopeService
	share    *ShareService
	sender   *channel.Sender
}

func NewPushService(
	repo *repository.PushRepository,
	userRepo *repository.UserRepository,
	roleRepo *repository.RoleRepository,
	orgRepo *repository.OrganizationRepository,
	dashRepo *repository.DashboardRepository,
	scope *ScopeService,
	share *ShareService,
	sender *channel.Sender,
) *PushService {
	return &PushService{
		repo: repo, userRepo: userRepo, roleRepo: roleRepo, orgRepo: orgRepo,
		dashRepo: dashRepo, scope: scope, share: share, sender: sender,
	}
}

func (s *PushService) ListSubscriptions() ([]SubscriptionView, error) {
	items, err := s.repo.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	result := make([]SubscriptionView, 0, len(items))
	for _, item := range items {
		result = append(result, s.toSubscriptionView(item))
	}
	return result, nil
}

func (s *PushService) CreateSubscription(input SubscriptionInput, userID uint64) (*SubscriptionView, error) {
	item, err := s.subscriptionFromInput(input, 0)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = userID
	if err := s.repo.CreateSubscription(item); err != nil {
		return nil, err
	}
	view := s.toSubscriptionView(*item)
	return &view, nil
}

func (s *PushService) UpdateSubscription(id uint64, input SubscriptionInput) (*SubscriptionView, error) {
	existing, err := s.repo.FindSubscription(id)
	if err != nil {
		return nil, err
	}
	item, err := s.subscriptionFromInput(input, id)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = existing.CreatedBy
	item.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateSubscription(item); err != nil {
		return nil, err
	}
	view := s.toSubscriptionView(*item)
	return &view, nil
}

func (s *PushService) DeleteSubscription(id uint64) error { return s.repo.DeleteSubscription(id) }

func (s *PushService) subscriptionFromInput(input SubscriptionInput, id uint64) (*model.Subscription, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("任务名称不能为空")
	}
	if input.DashboardID == 0 {
		return nil, errors.New("请选择推送看板")
	}
	if _, err := s.dashRepo.FindByID(input.DashboardID); err != nil {
		return nil, errors.New("推送看板不存在")
	}
	if _, err := cron.ParseStandard(input.Cron); err != nil {
		return nil, errors.New("Cron 表达式不正确，例如每天 9 点：0 9 * * *")
	}
	if len(input.Channels) == 0 {
		input.Channels = []string{"wecom"}
	}
	for _, item := range input.Channels {
		if item != "wecom" && item != "wechat" {
			return nil, errors.New("推送通道仅支持 wecom 或 wechat")
		}
	}
	if input.TemplateID != nil && *input.TemplateID > 0 {
		if _, err := s.repo.FindTemplate(*input.TemplateID); err != nil {
			return nil, errors.New("推送模板不存在")
		}
	}
	channels, _ := json.Marshal(input.Channels)
	recipients, _ := json.Marshal(input.Recipients)
	timezone := input.Timezone
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	item := &model.Subscription{
		ID: id, Name: input.Name, DashboardID: input.DashboardID, Cron: input.Cron,
		Timezone: timezone, Channels: string(channels), Recipients: string(recipients),
		TemplateID: input.TemplateID, Enabled: input.Enabled,
	}
	if item.Enabled {
		if schedule, err := cron.ParseStandard(item.Cron); err == nil {
			next := schedule.Next(time.Now())
			item.NextRunAt = &next
		}
	}
	return item, nil
}

func (s *PushService) toSubscriptionView(item model.Subscription) SubscriptionView {
	view := SubscriptionView{Subscription: item}
	_ = json.Unmarshal([]byte(item.Channels), &view.Channels)
	_ = json.Unmarshal([]byte(item.Recipients), &view.Recipients)
	if len(view.Channels) == 0 {
		view.Channels = []string{"wecom"}
	}
	if dash, err := s.dashRepo.FindByID(item.DashboardID); err == nil {
		view.DashboardName = dash.Name
	}
	if item.TemplateID != nil {
		if tpl, err := s.repo.FindTemplate(*item.TemplateID); err == nil {
			view.TemplateName = tpl.Name
		}
	}
	return view
}

func (s *PushService) ListTemplates() ([]model.PushTemplate, error) {
	return s.repo.ListTemplates()
}

func (s *PushService) CreateTemplate(input PushTemplateInput) (*model.PushTemplate, error) {
	if input.Name == "" || input.TitleTemplate == "" || input.ContentTemplate == "" {
		return nil, errors.New("模板名称、标题和内容不能为空")
	}
	if input.Channel != "wecom_text" && input.Channel != "wecom_textcard" && input.Channel != "wechat" {
		return nil, errors.New("模板通道类型不正确")
	}
	if input.StyleJSON != "" && !json.Valid([]byte(input.StyleJSON)) {
		return nil, errors.New("样式配置必须是合法 JSON")
	}
	item := &model.PushTemplate{
		Name: input.Name, Channel: input.Channel, TitleTemplate: input.TitleTemplate,
		ContentTemplate: input.ContentTemplate, StyleJSON: input.StyleJSON, Enabled: input.Enabled,
	}
	if err := s.repo.CreateTemplate(item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *PushService) UpdateTemplate(id uint64, input PushTemplateInput) (*model.PushTemplate, error) {
	item, err := s.repo.FindTemplate(id)
	if err != nil {
		return nil, err
	}
	if input.Name == "" || input.TitleTemplate == "" || input.ContentTemplate == "" {
		return nil, errors.New("模板名称、标题和内容不能为空")
	}
	if input.StyleJSON != "" && !json.Valid([]byte(input.StyleJSON)) {
		return nil, errors.New("样式配置必须是合法 JSON")
	}
	item.Name = input.Name
	item.Channel = input.Channel
	item.TitleTemplate = input.TitleTemplate
	item.ContentTemplate = input.ContentTemplate
	item.StyleJSON = input.StyleJSON
	item.Enabled = input.Enabled
	if err := s.repo.UpdateTemplate(item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *PushService) DeleteTemplate(id uint64) error { return s.repo.DeleteTemplate(id) }

func (s *PushService) ListRecords(limit int) ([]model.PushRecord, error) {
	return s.repo.ListRecords(limit)
}

func (s *PushService) ListChannelConfigs() ([]ChannelConfigView, error) {
	items, err := s.repo.ListChannelConfigs()
	if err != nil {
		return nil, err
	}
	result := make([]ChannelConfigView, 0, len(items))
	for _, item := range items {
		config := map[string]any{}
		_ = json.Unmarshal([]byte(item.ConfigJSON), &config)
		result = append(result, ChannelConfigView{
			Channel: item.Channel, Name: item.Name, Enabled: item.Enabled,
			Config: maskSecrets(config), UpdatedAt: item.UpdatedAt,
		})
	}
	return result, nil
}

func (s *PushService) SaveChannelConfig(channelName, name string, enabled bool, input map[string]any) (*ChannelConfigView, error) {
	if channelName != "wecom" && channelName != "wechat" {
		return nil, errors.New("不支持的推送通道")
	}
	var oldConfig map[string]any
	if existing, err := s.repo.FindChannelConfig(channelName); err == nil {
		oldConfig = map[string]any{}
		_ = json.Unmarshal([]byte(existing.ConfigJSON), &oldConfig)
	}
	for key, value := range input {
		if text, ok := value.(string); ok && (text == "" || strings.HasPrefix(text, "••")) {
			if oldConfig != nil {
				if old, exists := oldConfig[key]; exists {
					input[key] = old
				}
			}
		}
	}
	payload, _ := json.Marshal(input)
	item := &model.ChannelConfig{Channel: channelName, Name: name, ConfigJSON: string(payload), Enabled: enabled}
	if err := s.repo.UpsertChannelConfig(item); err != nil {
		return nil, err
	}
	configs, err := s.ListChannelConfigs()
	if err != nil {
		return nil, err
	}
	for _, cfg := range configs {
		if cfg.Channel == channelName {
			return &cfg, nil
		}
	}
	return &ChannelConfigView{Channel: channelName, Name: name, Enabled: enabled, Config: maskSecrets(input)}, nil
}

func (s *PushService) Preview(templateID uint64, dashboardID uint64) (*PushPreview, error) {
	tpl, err := s.repo.FindTemplate(templateID)
	if err != nil {
		return nil, errors.New("推送模板不存在")
	}
	dashboardName := "销售经营日报"
	if dashboardID > 0 {
		if dashboard, err := s.dashRepo.FindByID(dashboardID); err == nil {
			dashboardName = dashboard.Name
		}
	}
	values := sampleTemplateValues(dashboardName, "张店长")
	title, content, err := renderPushTemplate(tpl, values)
	if err != nil {
		return nil, err
	}
	style := map[string]any{}
	_ = json.Unmarshal([]byte(tpl.StyleJSON), &style)
	return &PushPreview{Title: title, Content: content, URL: "https://example.com/share/preview", Style: style}, nil
}

func (s *PushService) SendSubscription(ctx context.Context, subscriptionID uint64) (*SendSummary, error) {
	subscription, err := s.repo.FindSubscription(subscriptionID)
	if err != nil {
		return nil, errors.New("推送任务不存在")
	}
	return s.send(ctx, subscription)
}

func (s *PushService) SendByID(ctx context.Context, subscriptionID uint64) {
	if _, err := s.SendSubscription(ctx, subscriptionID); err != nil {
		_ = err
	}
}

// RetryRecord 重新执行失败记录所属的定时任务，生成一条新的推送结果。
func (s *PushService) RetryRecord(ctx context.Context, recordID uint64) (*SendSummary, error) {
	record, err := s.repo.FindRecord(recordID)
	if err != nil {
		return nil, errors.New("推送记录不存在")
	}
	if record.SubscriptionID == nil || *record.SubscriptionID == 0 {
		return nil, errors.New("该记录未关联定时任务，无法重试")
	}
	return s.SendSubscription(ctx, *record.SubscriptionID)
}

func (s *PushService) send(ctx context.Context, subscription *model.Subscription) (*SendSummary, error) {
	dashboard, err := s.dashRepo.FindByID(subscription.DashboardID)
	if err != nil {
		return nil, errors.New("看板不存在")
	}
	var rule RecipientRule
	_ = json.Unmarshal([]byte(subscription.Recipients), &rule)
	recipients, err := s.resolveRecipients(rule)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, errors.New("没有匹配到接收人")
	}
	var channels []string
	_ = json.Unmarshal([]byte(subscription.Channels), &channels)
	if len(channels) == 0 {
		channels = []string{"wecom"}
	}
	now := time.Now()
	startDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	endDate := startDate
	summary := &SendSummary{}

	for _, recipient := range recipients {
		scope, err := s.scope.Resolve(&recipient)
		if err != nil {
			summary.Failed++
			summary.Total++
			continue
		}
		role, _ := s.roleRepo.FindByCode(recipient.RoleCode)
		if !CanViewDashboard(&recipient, role, dashboard, scope.OrgIDs) {
			// 接收人不在看板可见范围内，跳过而不是发送越权链接
			summary.Skipped++
			summary.Total++
			continue
		}
		orgIDs := scope.OrgIDs
		if dashboard.ScopeMode == "all" {
			if all, err := s.orgRepo.AllIDs(); err == nil && len(all) > 0 {
				orgIDs = all
			}
		}
		stats, err := s.salesSummary(orgIDs, startDate, endDate)
		if err != nil {
			summary.Failed++
			summary.Total++
			continue
		}
		share, err := s.share.Create(subscription.CreatedBy, subscription.DashboardID, orgIDs, 24*7, 0, "")
		if err != nil {
			summary.Failed++
			summary.Total++
			continue
		}
		values := map[string]string{
			"DashboardName": dashboard.Name,
			"RecipientName": recipient.Name,
			"Period":        startDate,
			"Revenue":       formatMoney(stats.Revenue),
			"Orders":        fmt.Sprintf("%d", stats.Orders),
			"GrossProfit":   formatMoney(stats.GrossProfit),
			"Customers":     fmt.Sprintf("%d", stats.Customers),
			"URL":           share.URL,
		}
		for _, channelName := range channels {
			tpl, err := s.templateForChannel(subscription.TemplateID, channelName)
			if err != nil {
				summary.Failed++
				summary.Total++
				continue
			}
			title, content, err := renderPushTemplate(tpl, values)
			if err != nil {
				summary.Failed++
				summary.Total++
				continue
			}
			style := map[string]any{}
			_ = json.Unmarshal([]byte(tpl.StyleJSON), &style)
			buttonText, _ := style["button_text"].(string)
			accentColor, _ := style["accent_color"].(string)
			payload := channel.Payload{
				TemplateChannel: tpl.Channel, Title: title, Content: content,
				URL: share.URL, ButtonText: buttonText, AccentColor: accentColor,
			}
			cfg, err := s.repo.FindChannelConfig(channelName)
			if err != nil {
				cfg = &model.ChannelConfig{Channel: channelName, Enabled: false, ConfigJSON: "{}"}
			}
			result := s.sender.Send(ctx, *cfg, channel.Recipient{
				UserID: recipient.ID, Name: recipient.Name, WecomUserID: recipient.WecomUserID,
				WechatOpenID: recipient.WechatOpenID,
			}, payload)
			record := &model.PushRecord{
				SubscriptionID: &subscription.ID, DashboardID: &subscription.DashboardID,
				Channel: channelName, Recipient: recipient.Name, Status: result.Status,
				Title: title, Content: content, SentAt: time.Now(),
			}
			if result.Status == "failed" {
				record.Error = result.Message
			}
			_ = s.repo.CreateRecord(record)
			summary.Total++
			switch result.Status {
			case "sent":
				summary.Sent++
			case "simulated":
				summary.Simulated++
			case "skipped":
				summary.Skipped++
			default:
				summary.Failed++
			}
		}
	}
	status := "success"
	if summary.Failed > 0 {
		status = "failed"
	}
	if summary.Sent+summary.Simulated > 0 && summary.Failed > 0 {
		status = "partial"
	}
	summary.Status = status
	subscription.LastRunAt = &now
	subscription.LastStatus = status
	if schedule, err := cron.ParseStandard(subscription.Cron); err == nil {
		next := schedule.Next(time.Now())
		subscription.NextRunAt = &next
	}
	_ = s.repo.UpdateSubscription(subscription)
	return summary, nil
}

func (s *PushService) resolveRecipients(rule RecipientRule) ([]model.User, error) {
	byID := make(map[uint64]model.User)
	if len(rule.UserIDs) > 0 {
		users, err := s.userRepo.FindByIDs(rule.UserIDs)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			byID[user.ID] = user
		}
	}
	var orgIDs []uint64
	if len(rule.OrgIDs) > 0 {
		seen := map[uint64]bool{}
		for _, orgID := range rule.OrgIDs {
			ids, err := s.orgRepo.DescendantIDs(orgID)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				if !seen[id] {
					seen[id] = true
					orgIDs = append(orgIDs, id)
				}
			}
		}
	}
	if len(rule.Roles) > 0 || len(rule.OrgIDs) > 0 {
		users, err := s.userRepo.FindRecipients(rule.Roles, orgIDs)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			byID[user.ID] = user
		}
	}
	if len(rule.UserIDs) == 0 && len(rule.Roles) == 0 && len(rule.OrgIDs) == 0 {
		users, err := s.userRepo.FindRecipients(nil, nil)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			byID[user.ID] = user
		}
	}
	result := make([]model.User, 0, len(byID))
	for _, user := range byID {
		result = append(result, user)
	}
	return result, nil
}

func (s *PushService) templateForChannel(templateID *uint64, channelName string) (*model.PushTemplate, error) {
	if templateID != nil {
		tpl, err := s.repo.FindTemplate(*templateID)
		if err == nil {
			if strings.HasPrefix(tpl.Channel, channelName) || channelName == "wechat" && tpl.Channel == "wechat" {
				return tpl, nil
			}
		}
	}
	templates, err := s.repo.ListTemplates()
	if err != nil {
		return nil, err
	}
	for i := range templates {
		if templates[i].Enabled && strings.HasPrefix(templates[i].Channel, channelName) {
			return &templates[i], nil
		}
	}
	return nil, fmt.Errorf("%s 没有可用的推送模板", channelName)
}

type salesStats struct {
	Revenue     float64
	Orders      int
	GrossProfit float64
	Customers   int
}

func (s *PushService) salesSummary(orgIDs []uint64, startDate, endDate string) (salesStats, error) {
	var stats salesStats
	if len(orgIDs) == 0 {
		return stats, nil
	}
	err := s.repo.DB().Model(&model.SalesFact{}).
		Select("COALESCE(SUM(revenue),0) AS revenue, COALESCE(SUM(orders),0) AS orders, COALESCE(SUM(gross_profit),0) AS gross_profit, COALESCE(SUM(customers),0) AS customers").
		Where("org_id IN ? AND stat_date BETWEEN ? AND ?", orgIDs, startDate, endDate).
		Scan(&stats).Error
	return stats, err
}

func renderPushTemplate(tpl *model.PushTemplate, values map[string]string) (string, string, error) {
	parse := func(name, source string) (string, error) {
		parsed, err := template.New(name).Option("missingkey=zero").Parse(source)
		if err != nil {
			return "", fmt.Errorf("模板语法错误: %w", err)
		}
		var builder strings.Builder
		if err := parsed.Execute(&builder, values); err != nil {
			return "", fmt.Errorf("模板渲染失败: %w", err)
		}
		return builder.String(), nil
	}
	title, err := parse("title", tpl.TitleTemplate)
	if err != nil {
		return "", "", err
	}
	content, err := parse("content", tpl.ContentTemplate)
	if err != nil {
		return "", "", err
	}
	return title, content, nil
}

func sampleTemplateValues(dashboardName, recipientName string) map[string]string {
	return map[string]string{
		"DashboardName": dashboardName,
		"RecipientName": recipientName,
		"Period":        time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
		"Revenue":       "128,560.00",
		"Orders":        "328",
		"GrossProfit":   "47,320.00",
		"Customers":     "291",
		"URL":           "https://example.com/share/preview",
	}
}

func formatMoney(value float64) string {
	raw := fmt.Sprintf("%.2f", value)
	parts := strings.Split(raw, ".")
	integer := parts[0]
	negative := strings.HasPrefix(integer, "-")
	if negative {
		integer = strings.TrimPrefix(integer, "-")
	}
	chunks := make([]string, 0)
	for len(integer) > 3 {
		chunks = append([]string{integer[len(integer)-3:]}, chunks...)
		integer = integer[:len(integer)-3]
	}
	chunks = append([]string{integer}, chunks...)
	result := strings.Join(chunks, ",")
	if len(parts) > 1 {
		result += "." + parts[1]
	}
	if negative {
		result = "-" + result
	}
	return result
}

func maskSecrets(config map[string]any) map[string]any {
	result := make(map[string]any, len(config))
	for key, value := range config {
		if strings.Contains(strings.ToLower(key), "secret") {
			if text, ok := value.(string); ok && text != "" {
				result[key] = "••••••••"
				continue
			}
		}
		result[key] = value
	}
	return result
}

var _ = gorm.ErrRecordNotFound
