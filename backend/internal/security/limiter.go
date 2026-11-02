package security

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu          sync.Mutex
	entries     map[string]*limiterEntry
	rpm         int
	burst       int
	idleTimeout time.Duration
	lastCleanup time.Time
}

func NewRateLimiter(rpm, burst, idleMinutes int) *RateLimiter {
	if rpm <= 0 {
		rpm = 10
	}
	if burst <= 0 {
		burst = 3
	}
	if idleMinutes <= 0 {
		idleMinutes = 30
	}
	return &RateLimiter{
		entries:     make(map[string]*limiterEntry),
		rpm:         rpm,
		burst:       burst,
		idleTimeout: time.Duration(idleMinutes) * time.Minute,
		lastCleanup: time.Now(),
	}
}

func (l *RateLimiter) Allow(principalID string) (bool, int) {
	if principalID == "" {
		principalID = "anonymous"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	// 定期清理闲置限流器（每 5 分钟检查一次）
	if now.Sub(l.lastCleanup) > 5*time.Minute {
		l.cleanupLocked(now)
		l.lastCleanup = now
	}

	entry, exists := l.entries[principalID]
	if !exists {
		limit := rate.Limit(float64(l.rpm) / 60.0)
		entry = &limiterEntry{
			limiter:  rate.NewLimiter(limit, l.burst),
			lastSeen: now,
		}
		l.entries[principalID] = entry
	}
	entry.lastSeen = now

	res := entry.limiter.Reserve()
	if !res.OK() {
		return false, 1
	}
	delay := res.Delay()
	if delay > 0 {
		res.Cancel()
		retrySecs := int(math.Ceil(delay.Seconds()))
		if retrySecs <= 0 {
			retrySecs = 1
		}
		return false, retrySecs
	}

	return true, 0
}

func (l *RateLimiter) cleanupLocked(now time.Time) {
	for id, entry := range l.entries {
		if now.Sub(entry.lastSeen) > l.idleTimeout {
			delete(l.entries, id)
		}
	}
}
