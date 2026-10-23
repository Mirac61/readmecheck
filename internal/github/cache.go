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
	mu         sync.RWMutex
	entries    map[string]cacheEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newSnapshotCache(ttl time.Duration, now func() time.Time) *snapshotCache {
	return &snapshotCache{
		entries:    make(map[string]cacheEntry),
		ttl:        ttl,
		maxEntries: DefaultCacheMaxEntries,
		now:        now,
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
	defer c.mu.Unlock()
	if c.maxEntries > 0 && len(c.entries) >= c.maxEntries {
		c.evictLocked()
	}
	c.entries[key] = cacheEntry{
		repository: repository,
		expiresAt:  c.now().Add(c.ttl),
	}
}

// evictLocked drops expired entries first and then the entry that expires
// soonest, so the cache stays bounded without an unbounded scan.
func (c *snapshotCache) evictLocked() {
	now := c.now()
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	if c.maxEntries <= 0 || len(c.entries) < c.maxEntries {
		return
	}
	oldestKey := ""
	var oldest time.Time
	for key, entry := range c.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldest) {
			oldestKey, oldest = key, entry.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *snapshotCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
