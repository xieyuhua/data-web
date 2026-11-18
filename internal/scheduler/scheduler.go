package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // IANA timezone database for Windows

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"biaoge-bi/internal/config"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/service"
)

type ReportBuilder func(ctx context.Context, dashboardID uint) ([]byte, string, error)

type Scheduler struct {
	cfg     *config.Config
	runtime *config.Runtime
	meta    *gorm.DB
	cron    *cron.Cron
	builder ReportBuilder
	mailer  *Mailer
	wecom   *WeComClient

	mu      sync.Mutex
	entries map[uint]cron.EntryID
	running sync.Map // taskID -> struct{}
}

// SetRuntime lets SMTP/WeCom/retention settings change without a restart.
func (s *Scheduler) SetRuntime(rt *config.Runtime) { s.runtime = rt }

// cfgNow returns the live config snapshot.
func (s *Scheduler) cfgNow() *config.Config {
	if s.runtime != nil {
		return s.runtime.Get()
	}
	return s.cfg
}

func New(cfg *config.Config, meta *gorm.DB, builder ReportBuilder) *Scheduler {
	s := &Scheduler{
		cfg:     cfg,
		meta:    meta,
		cron:    cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger), cron.SkipIfStillRunning(cron.DefaultLogger)), cron.WithSeconds()),
		builder: builder,
		mailer:  NewMailer(&cfg.SMTP),
		wecom:   NewWeComClient(cfg.WeCom),
		entries: make(map[uint]cron.EntryID),
	}
	return s
}

func (s *Scheduler) Start() {
	if !s.cfg.Scheduler.Enabled {
		slog.Info("定时报表调度器未启用")
		return
	}
	if err := os.MkdirAll(s.cfg.Scheduler.ReportDir, 0o755); err != nil {
		slog.Error("创建报表目录失败", "error", err)
	}

	var tasks []model.ScheduleTask
	if err := s.meta.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		slog.Error("加载定时任务失败", "error", err)
	}
	s.mu.Lock()
	for _, t := range tasks {
		if err := s.registerLocked(t); err != nil {
			slog.Error("注册定时任务失败", "task", t.ID, "error", err)
		}
	}
	s.mu.Unlock()
	s.cron.Start()
	go s.cleanupLoop()
	slog.Info("定时报表调度器已启动", "tasks", len(tasks))
}

func (s *Scheduler) Stop() {
	if s.cron != nil {
		ctx := s.cron.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// Reload re-registers a single task (after create/update) or removes it (disabled/deleted).
func (s *Scheduler) Reload(taskID uint) {
	var t model.ScheduleTask
	err := s.meta.First(&t, taskID).Error
	s.mu.Lock()
	defer s.mu.Unlock()
	if eid, ok := s.entries[taskID]; ok {
		s.cron.Remove(eid)
		delete(s.entries, taskID)
	}
	if err != nil {
		return
	}
	if !t.Enabled || !s.cfg.Scheduler.Enabled {
		return
	}
	if err := s.registerLocked(t); err != nil {
		slog.Error("注册定时任务失败", "task", taskID, "error", err)
	}
}

// registerLocked adds a cron entry. The caller must hold s.mu.
func (s *Scheduler) registerLocked(t model.ScheduleTask) error {
	spec := t.CronExpr
	if t.Timezone != "" && strings.ToUpper(t.Timezone) != "LOCAL" {
		spec = "CRON_TZ=" + t.Timezone + " " + spec
	}
	task := t
	eid, err := s.cron.AddFunc(spec, func() { s.run(&task, "cron") })
	if err != nil {
		return err
	}
	s.entries[task.ID] = eid
	// Entry() asks the cron scheduler, which is safe here because cron never
	// calls back into Scheduler while holding its own lock.
	s.updateNextRun(task.ID, eid)
	return nil
}

func (s *Scheduler) updateNextRun(taskID uint, eid cron.EntryID) {
	entry := s.cron.Entry(eid)
	if entry.Next.IsZero() {
		return
	}
	next := entry.Next
	_ = s.meta.Model(&model.ScheduleTask{}).Where("id = ?", taskID).Update("next_run_at", &next)
}

// RunNow triggers a task immediately (manual run).
func (s *Scheduler) RunNow(taskID uint) (*model.ScheduleRun, error) {
	t, err := s.loadTask(taskID)
	if err != nil {
		return nil, err
	}
	return s.run(t, "manual")
}

func (s *Scheduler) loadTask(id uint) (*model.ScheduleTask, error) {
	var t model.ScheduleTask
	if err := s.meta.First(&t, id).Error; err != nil {
		return nil, errors.New("定时任务不存在")
	}
	return &t, nil
}

func (s *Scheduler) run(t *model.ScheduleTask, triggeredBy string) (*model.ScheduleRun, error) {
	if _, busy := s.running.LoadOrStore(t.ID, struct{}{}); busy {
		return nil, errors.New("任务正在执行中")
	}
	defer s.running.Delete(t.ID)

	run := model.ScheduleRun{
		TaskID: t.ID, TriggeredBy: triggeredBy, StartedAt: time.Now(), Status: "running",
	}
	if err := s.meta.Create(&run).Error; err != nil {
		return nil, err
	}

	data, filename, err := s.build(t)
	if err != nil {
		s.finish(&run, t.ID, "failed", err.Error(), "", 0)
		return &run, err
	}

	dir := filepath.Join(s.cfg.Scheduler.ReportDir, strconv.FormatUint(uint64(t.ID), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.finish(&run, t.ID, "failed", err.Error(), "", 0)
		return &run, err
	}
	path := filepath.Join(dir, fmt.Sprintf("run-%d-%s", run.ID, filename))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.finish(&run, t.ID, "failed", err.Error(), "", 0)
		return &run, err
	}

	var recipients []string
	_ = json.Unmarshal([]byte(t.RecipientsJSON), &recipients)

	cfgNow := s.cfgNow()
	mailer := NewMailer(&cfgNow.SMTP)
	wecomClient := NewWeComClient(cfgNow.WeCom)

	var sendErr error
	switch t.DeliveryChannel {
	case "wecom":
		sendErr = s.pushWeComWith(wecomClient, t, recipients)
	case "email", "":
		if !mailer.Enabled() {
			sendErr = errors.New("SMTP 未配置，报表已生成但未发送")
		} else {
			subject := fmt.Sprintf("【BI 报表】%s %s", t.Name, time.Now().Format("2006-01-02 15:04"))
			body := fmt.Sprintf("您好，附件为「%s」的定时报表，生成时间 %s。", t.Name, time.Now().Format("2006-01-02 15:04:05"))
			sendErr = mailer.SendReport(recipients, subject, body, path)
		}
	default:
		sendErr = fmt.Errorf("不支持的推送渠道 %q", t.DeliveryChannel)
	}

	if sendErr != nil {
		s.finish(&run, t.ID, "failed", sendErr.Error(), path, fileSize(path))
		return &run, sendErr
	}
	s.finish(&run, t.ID, "success", "", path, fileSize(path))
	return &run, nil
}

// WeComLinkBuilder issues a time-limited public share URL for a dashboard.
// Wired by main so the scheduler does not depend on the share service directly.
type WeComLinkBuilder func(dashboardID uint, expireHours int) (string, error)

// SummaryBuilder returns a short line of key figures for the alert card.
type SummaryBuilder func(ctx context.Context, dashboardID uint) ([]string, error)

var (
	wecomLinkBuilder  WeComLinkBuilder
	wecomSummaryBuild SummaryBuilder
)

func SetWeComHooks(link WeComLinkBuilder, summary SummaryBuilder) {
	wecomLinkBuilder = link
	wecomSummaryBuild = summary
}

// pushWeCom sends the report as a WeCom application message: a textcard with
// key figures plus a time-limited link, or a plain text message.
func (s *Scheduler) pushWeComWith(client *WeComClient, t *model.ScheduleTask, recipients []string) error {
	if !client.Enabled() {
		return errors.New("企业微信未配置，无法推送")
	}
	if len(recipients) == 0 {
		return errors.New("企业微信收件人不能为空")
	}
	if wecomLinkBuilder == nil {
		return errors.New("分享链接服务未初始化")
	}
	expireHours := t.LinkExpireHours
	if expireHours <= 0 {
		expireHours = 72
	}
	link, err := wecomLinkBuilder(t.DashboardID, expireHours)
	if err != nil {
		return fmt.Errorf("生成分享链接失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	lines := []string{}
	if wecomSummaryBuild != nil {
		lines, _ = wecomSummaryBuild(ctx, t.DashboardID)
	}

	body := strings.Join(lines, "\n")
	if body == "" {
		body = fmt.Sprintf("报表生成时间：%s", time.Now().Format("2006-01-02 15:04"))
	}
	expireText := "永久有效"
	if expireHours > 0 {
		expireText = fmt.Sprintf("%d 小时内有效", expireHours)
	}

	if t.MessageFormat == "text" {
		content := fmt.Sprintf("【%s】%s\n%s\n\n查看报表：%s\n链接%s",
			t.Name, time.Now().Format("2006-01-02 15:04"), body, link, expireText)
		return client.SendText(ctx, recipients, content)
	}
	card := wecomTextCard{
		Title:       fmt.Sprintf("%s · %s", t.Name, time.Now().Format("01-02 15:04")),
		Description: fmt.Sprintf("<div class=\"gray\">%s</div><div class=\"normal\">%s</div>", strings.ReplaceAll(body, "\n", "<br/>"), expireText),
		URL:         link,
		BtnText:     "查看报表",
	}
	return client.SendTextCard(ctx, recipients, card)
}

func (s *Scheduler) build(t *model.ScheduleTask) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	data, name, err := s.builder(ctx, t.DashboardID)
	if err != nil {
		return nil, "", err
	}
	if name == "" {
		name = "report.xlsx"
	}
	return data, name, nil
}

func (s *Scheduler) finish(run *model.ScheduleRun, taskID uint, status, errMsg, path string, size int64) {
	now := time.Now()
	updates := map[string]any{
		"finished_at":    &now,
		"status":         status,
		"error_message":  truncateStr(errMsg, 1000),
		"file_path":      path,
		"file_size":      size,
	}
	_ = s.meta.Model(&model.ScheduleRun{}).Where("id = ?", run.ID).Updates(updates).Error
	run.FinishedAt = &now
	run.Status = status
	run.ErrorMessage = errMsg
	run.FilePath = path
	run.FileSize = size

	if status == "success" {
		_ = s.meta.Model(&model.ScheduleTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"last_run_status": "success", "last_run_at": &now,
		})
	} else {
		_ = s.meta.Model(&model.ScheduleTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"last_run_status": "failed", "last_run_at": &now,
		})
	}
}

func (s *Scheduler) cleanupLoop() {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for range t.C {
		s.cleanup()
	}
}

func (s *Scheduler) cleanup() {
	days := s.cfg.Scheduler.RetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var runs []model.ScheduleRun
	if err := s.meta.Where("created_at < ? AND file_path <> ?", cutoff, "").Find(&runs).Error; err != nil {
		return
	}
	for _, r := range runs {
		if r.FilePath != "" {
			_ = os.Remove(r.FilePath)
		}
	}
	_ = s.meta.Where("created_at < ?", cutoff).Delete(&model.ScheduleRun{}).Error
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Provide a default builder backed by the export service.
func NewBuilder(exportSvc *service.ExportService) ReportBuilder {
	return func(ctx context.Context, dashboardID uint) ([]byte, string, error) {
		return exportSvc.DashboardXLSX(ctx, dashboardID, service.DashboardDataRequest{})
	}
}
