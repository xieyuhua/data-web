package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"bi-platform/internal/repository"
	"bi-platform/internal/service"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron *cron.Cron
	repo *repository.PushRepository
	push *service.PushService
	mu   sync.Mutex // 保护 Reload 期间的并发重载
}

// New 创建调度器；时区非法时回退到服务器本地时区。
func New(repo *repository.PushRepository, push *service.PushService, timezone string) *Scheduler {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.Local
	}
	return &Scheduler{
		// Recover 链保证单个任务 panic 不影响整个调度器
		cron: cron.New(cron.WithLocation(location), cron.WithChain(cron.Recover(cron.DefaultLogger))),
		repo: repo,
		push: push,
	}
}

// Start 启动 cron 并加载已有订阅。
func (s *Scheduler) Start() {
	s.cron.Start()
	s.Reload()
}

// Stop 停止调度器，最多等待 5 秒让运行中的任务收尾。
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
}

// Reload 重新加载订阅：先清空所有任务，再按数据库中的启用状态重新注册。
// 订阅变更（新增、修改、启停、删除）后需要调用该方法。
func (s *Scheduler) Reload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 清空旧任务，避免重复注册
	for _, entry := range s.cron.Entries() {
		s.cron.Remove(entry.ID)
	}
	subscriptions, err := s.repo.ListSubscriptions()
	if err != nil {
		log.Printf("scheduler: load subscriptions failed: %v", err)
		return
	}
	for i := range subscriptions {
		sub := subscriptions[i]
		if !sub.Enabled {
			continue
		}
		// 校验 cron 表达式，非法表达式只记录日志，不影响其它订阅
		schedule, err := cron.ParseStandard(sub.Cron)
		if err != nil {
			log.Printf("scheduler: invalid cron for subscription %d: %v", sub.ID, err)
			continue
		}
		// 记录下次执行时间，便于前端展示
		next := schedule.Next(time.Now())
		sub.NextRunAt = &next
		_ = s.repo.UpdateSubscription(&sub)
		subscriptionID := sub.ID
		_, err = s.cron.AddFunc(sub.Cron, func() {
			// 单任务兜底 recover，避免 panic 导致调度器退出
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("scheduler: panic in subscription %d: %v", subscriptionID, recovered)
				}
			}()
			log.Printf("scheduler: running subscription %d", subscriptionID)
			// 使用后台上下文执行推送，不依赖任何 HTTP 请求
			s.push.SendByID(context.Background(), subscriptionID)
		})
		if err != nil {
			log.Printf("scheduler: register subscription %d failed: %v", subscriptionID, err)
		}
	}
	log.Printf("scheduler: %d enabled subscriptions loaded", len(s.cron.Entries()))
}
