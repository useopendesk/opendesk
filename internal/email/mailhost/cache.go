package mailhost

import (
	"context"
	"sync"
	"time"
)

// MemoryCache keeps detections in memory for a while, bounded in size.
type MemoryCache struct {
	ttl time.Duration
	max int
	now func() time.Time

	mu sync.Mutex
	m  map[string]cached
}

type cached struct {
	d       Detection
	expires time.Time
}

// NewMemoryCache keeps up to max detections for ttl each.
func NewMemoryCache(ttl time.Duration, max int) *MemoryCache {
	return &MemoryCache{ttl: ttl, max: max, now: time.Now, m: map[string]cached{}}
}

func (c *MemoryCache) Get(_ context.Context, domain string) (Detection, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[domain]
	if !ok {
		return Detection{}, false
	}
	if !c.now().Before(e.expires) {
		delete(c.m, domain)
		return Detection{}, false
	}
	return e.d, true
}

func (c *MemoryCache) Set(_ context.Context, domain string, d Detection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.m) >= c.max {
		// Drop what has expired; if that frees nothing, start over.
		for k, e := range c.m {
			if !now.Before(e.expires) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= c.max {
			c.m = map[string]cached{}
		}
	}
	c.m[domain] = cached{d: d, expires: now.Add(c.ttl)}
}
