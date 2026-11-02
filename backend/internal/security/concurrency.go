package security

import (
	"sync"
)

type ConcurrencyManager struct {
	mu                 sync.Mutex
	globalLimit        int
	perPrincipal       int
	globalCount        int
	perPrincipalCounts map[string]int
}

func NewConcurrencyManager(globalLimit, perPrincipal int) *ConcurrencyManager {
	if globalLimit <= 0 {
		globalLimit = 2
	}
	if perPrincipal <= 0 {
		perPrincipal = 1
	}
	return &ConcurrencyManager{
		globalLimit:        globalLimit,
		perPrincipal:       perPrincipal,
		perPrincipalCounts: make(map[string]int),
	}
}

func (c *ConcurrencyManager) Acquire(principalID string) (func(), bool) {
	if principalID == "" {
		principalID = "anonymous"
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.globalCount >= c.globalLimit {
		return nil, false
	}
	if c.perPrincipalCounts[principalID] >= c.perPrincipal {
		return nil, false
	}

	c.globalCount++
	c.perPrincipalCounts[principalID]++

	var once sync.Once
	release := func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			c.globalCount--
			if c.globalCount < 0 {
				c.globalCount = 0
			}

			c.perPrincipalCounts[principalID]--
			if c.perPrincipalCounts[principalID] <= 0 {
				delete(c.perPrincipalCounts, principalID)
			}
		})
	}

	return release, true
}
