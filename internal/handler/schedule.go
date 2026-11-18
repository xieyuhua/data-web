package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
	"biaoge-bi/internal/service"
	"biaoge-bi/internal/web"
)

// SchedulerHooks are wired by main to avoid an import cycle with the scheduler package.
var SchedulerHooks struct {
	RunNow func(taskID uint) (*model.ScheduleRun, error)
	Reload func(taskID uint)
}

type ScheduleHandler struct {
	repo      *repository.Repo
	dashboard *service.DashboardService
}

func NewScheduleHandler(repo *repository.Repo, dashboard *service.DashboardService) *ScheduleHandler {
	return &ScheduleHandler{repo: repo, dashboard: dashboard}
}

type schedulePayload struct {
	Name            string   `json:"name"`
	DashboardID     uint     `json:"dashboardId"`
	CronExpr        string   `json:"cronExpr"`
	Timezone        string   `json:"timezone"`
	Enabled         bool     `json:"enabled"`
	DeliveryChannel string   `json:"deliveryChannel"`
	MessageFormat   string   `json:"messageFormat"`
	LinkExpireHours int      `json:"linkExpireHours"`
	Recipients      []string `json:"recipients"`
}

func (h *ScheduleHandler) List(c *gin.Context) {
	var list []model.ScheduleTask
	q := h.repo.DB.Order("id DESC")
	u := web.CurrentUser(c)
	if !u.IsAdmin() {
		q = q.Where("created_by_id = ?", u.ID)
	}
	if err := q.Find(&list).Error; err != nil {
		web.ServerError(c, "查询定时任务失败")
		return
	}
	web.OK(c, list)
}

func (h *ScheduleHandler) Create(c *gin.Context) {
	var p schedulePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || p.DashboardID == 0 || strings.TrimSpace(p.CronExpr) == "" {
		web.BadRequest(c, "名称、仪表盘和 cron 表达式不能为空")
		return
	}
	if _, err := h.dashboard.Get(p.DashboardID, web.CurrentUser(c)); err != nil {
		writeDashboardError(c, err)
		return
	}
	if p.DeliveryChannel == "" {
		p.DeliveryChannel = "email"
	}
	switch p.DeliveryChannel {
	case "email", "wecom":
	default:
		web.BadRequest(c, "不支持的推送渠道")
		return
	}
	if p.MessageFormat == "" {
		p.MessageFormat = "card"
	}
	if p.MessageFormat != "card" && p.MessageFormat != "text" {
		web.BadRequest(c, "消息格式只能是卡片或文本")
		return
	}
	if p.LinkExpireHours < 0 || p.LinkExpireHours > 720 {
		web.BadRequest(c, "链接有效期需在 0-720 小时之间")
		return
	}
	if p.LinkExpireHours == 0 {
		p.LinkExpireHours = 72
	}
	if len(p.Recipients) == 0 {
		web.BadRequest(c, "请填写接收人")
		return
	}
	if p.Timezone == "" {
		p.Timezone = "Local"
	}
	if err := validateCron(p.CronExpr, p.Timezone); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	recipients, _ := json.Marshal(p.Recipients)
	t := model.ScheduleTask{
		Name: p.Name, DashboardID: p.DashboardID, CronExpr: p.CronExpr, Timezone: p.Timezone,
		Enabled: p.Enabled, DeliveryChannel: p.DeliveryChannel, Format: "xlsx",
		MessageFormat: p.MessageFormat, LinkExpireHours: p.LinkExpireHours,
		RecipientsJSON: string(recipients), CreatedByID: web.CurrentUser(c).ID,
	}
	if err := h.repo.DB.Create(&t).Error; err != nil {
		web.ServerError(c, "创建定时任务失败")
		return
	}
	h.reload(t.ID)
	web.OK(c, t)
}

func (h *ScheduleHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	t, err := h.repo.ScheduleByID(id)
	if err != nil {
		web.NotFound(c, "定时任务不存在")
		return
	}
	var p schedulePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		web.BadRequest(c, "参数错误")
		return
	}
	if err := validateCron(p.CronExpr, p.Timezone); err != nil {
		web.BadRequest(c, err.Error())
		return
	}
	t.Name = p.Name
	t.CronExpr = p.CronExpr
	if p.Timezone != "" {
		t.Timezone = p.Timezone
	}
	t.Enabled = p.Enabled
	if p.DeliveryChannel != "" {
		switch p.DeliveryChannel {
		case "email", "wecom":
		default:
			web.BadRequest(c, "不支持的推送渠道")
			return
		}
		t.DeliveryChannel = p.DeliveryChannel
	}
	if p.MessageFormat != "" {
		if p.MessageFormat != "card" && p.MessageFormat != "text" {
			web.BadRequest(c, "消息格式只能是卡片或文本")
			return
		}
		t.MessageFormat = p.MessageFormat
	}
	if p.LinkExpireHours > 0 {
		if p.LinkExpireHours > 720 {
			web.BadRequest(c, "链接有效期需在 0-720 小时之间")
			return
		}
		t.LinkExpireHours = p.LinkExpireHours
	}
	if p.Recipients != nil {
		b, _ := json.Marshal(p.Recipients)
		t.RecipientsJSON = string(b)
	}
	if err := h.repo.DB.Save(t).Error; err != nil {
		web.ServerError(c, "更新定时任务失败")
		return
	}
	h.reload(t.ID)
	web.OK(c, t)
}

func (h *ScheduleHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.repo.DB.Delete(&model.ScheduleTask{}, id).Error; err != nil {
		web.ServerError(c, "删除定时任务失败")
		return
	}
	h.reload(id)
	web.OK(c, gin.H{"message": "已删除"})
}

func (h *ScheduleHandler) RunNow(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if SchedulerHooks.RunNow == nil {
		web.ServerError(c, "调度器未初始化")
		return
	}
	run, err := SchedulerHooks.RunNow(id)
	if err != nil {
		web.Unprocessable(c, err.Error())
		return
	}
	web.OK(c, run)
}

func (h *ScheduleHandler) Runs(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	runs, err := h.repo.ListScheduleRuns(id, 50)
	if err != nil {
		web.ServerError(c, "查询执行记录失败")
		return
	}
	web.OK(c, runs)
}

func (h *ScheduleHandler) DownloadRun(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	run, err := h.repo.ScheduleRunByID(id)
	if err != nil {
		web.NotFound(c, "执行记录不存在")
		return
	}
	if run.FilePath == "" {
		web.NotFound(c, "报表文件不存在")
		return
	}
	if _, err := os.Stat(run.FilePath); err != nil {
		web.NotFound(c, "报表文件已被清理")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(run.FilePath)))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.File(run.FilePath)
}

func (h *ScheduleHandler) reload(taskID uint) {
	if SchedulerHooks.Reload != nil {
		SchedulerHooks.Reload(taskID)
	}
}

var cronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// validateCron rejects malformed cron expressions and unknown timezones early.
func validateCron(expr, timezone string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("Cron 表达式不能为空")
	}
	if _, err := cronParser.Parse(expr); err != nil {
		return fmt.Errorf("Cron 表达式无效: %v", err)
	}
	if timezone != "" && !strings.EqualFold(timezone, "local") {
		if _, err := time.LoadLocation(timezone); err != nil {
			return fmt.Errorf("时区无效: %s", timezone)
		}
	}
	return nil
}
