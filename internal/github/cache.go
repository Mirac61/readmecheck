package github

import (
	"sync"
	"time"
)

type cacheEntry struct {
	repository *Repository
	expiresAt  time.Time
}

type snapshotCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	now     func() time.Time
}

func newSnapshotCache(ttl time.Duration, now func() time.Time) *snapshotCache {
	return &snapshotCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     now,
	}
}

func (c *snapshotCache) get(key string) (*Repository, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.repository, true
}

func (c *snapshotCache) put(key string, repository *Repository) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{
		repository: repository,
		expiresAt:  c.now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *snapshotCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
