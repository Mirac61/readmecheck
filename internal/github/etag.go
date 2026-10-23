package github

import "sync"

const (
	etagMaxEntries    = 64
	etagMaxBodyBytes  = 1 << 20
	etagMaxTotalBytes = 8 << 20
)

type etagEntry struct {
	etag string
	body []byte
	// meta carries a response detail that a 304 does not repeat, such as the
	// README filename from Content-Disposition.
	meta string
}

// etagCache stores response bodies keyed by request URL so a conditional
// request can reuse them after GitHub answers 304 Not Modified. Bodies are
// bounded because a tree response can be large.
type etagCache struct {
	mu         sync.Mutex
	entries    map[string]etagEntry
	order      []string
	totalBytes int
}

func newETagCache() *etagCache {
	return &etagCache{entries: make(map[string]etagEntry)}
}

func (c *etagCache) lookup(target string) (etagEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[target]
	return entry, ok
}

func (c *etagCache) delete(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[target]
	if !ok {
		return
	}
	delete(c.entries, target)
	c.totalBytes -= len(entry.body)
	for i, key := range c.order {
		if key == target {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (c *etagCache) store(target, etag, meta string, body []byte) {
	if etag == "" || len(body) == 0 || len(body) > etagMaxBodyBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if previous, ok := c.entries[target]; ok {
		c.totalBytes -= len(previous.body)
		for i, key := range c.order {
			if key == target {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}
	for len(c.entries) >= etagMaxEntries ||
		(c.totalBytes+len(body) > etagMaxTotalBytes && len(c.order) > 0) {
		oldest := c.order[0]
		c.order = c.order[1:]
		entry := c.entries[oldest]
		delete(c.entries, oldest)
		c.totalBytes -= len(entry.body)
	}
	c.entries[target] = etagEntry{etag: etag, body: body, meta: meta}
	c.order = append(c.order, target)
	c.totalBytes += len(body)
}

func (c *etagCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
