package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter is a keyed in-memory rate limiter with idle-entry cleanup.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	r       rate.Limit
	burst   int
	janitor *time.Ticker
	stop    chan struct{}
}

func NewLimiter(perMinute int, burst int) *Limiter {
	l := &Limiter{
		entries: make(map[string]*entry),
		r:       rate.Limit(float64(perMinute) / 60.0),
		burst:   burst,
		janitor: time.NewTicker(5 * time.Minute),
		stop:    make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.r, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = time.Now()
	l.mu.Unlock()
	return e.limiter.Allow()
}

func (l *Limiter) cleanup() {
	for {
		select {
		case <-l.janitor.C:
			cutoff := time.Now().Add(-30 * time.Minute)
			l.mu.Lock()
			for k, e := range l.entries {
				if e.lastSeen.Before(cutoff) {
					delete(l.entries, k)
				}
			}
			l.mu.Unlock()
		case <-l.stop:
			return
		}
	}
}

func (l *Limiter) Close() {
	l.janitor.Stop()
	close(l.stop)
}
