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
	mu   sync.Mutex
}

func New(repo *repository.PushRepository, push *service.PushService, timezone string) *Scheduler {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.Local
	}
	return &Scheduler{
		cron: cron.New(cron.WithLocation(location), cron.WithChain(cron.Recover(cron.DefaultLogger))),
		repo: repo,
		push: push,
	}
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.Reload()
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
}

func (s *Scheduler) Reload() {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		schedule, err := cron.ParseStandard(sub.Cron)
		if err != nil {
			log.Printf("scheduler: invalid cron for subscription %d: %v", sub.ID, err)
			continue
		}
		next := schedule.Next(time.Now())
		sub.NextRunAt = &next
		_ = s.repo.UpdateSubscription(&sub)
		subscriptionID := sub.ID
		_, err = s.cron.AddFunc(sub.Cron, func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("scheduler: panic in subscription %d: %v", subscriptionID, recovered)
				}
			}()
			log.Printf("scheduler: running subscription %d", subscriptionID)
			s.push.SendByID(context.Background(), subscriptionID)
		})
		if err != nil {
			log.Printf("scheduler: register subscription %d failed: %v", subscriptionID, err)
		}
	}
	log.Printf("scheduler: %d enabled subscriptions loaded", len(s.cron.Entries()))
}
