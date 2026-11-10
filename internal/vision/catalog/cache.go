package catalog

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type snapshotFetcher interface {
	Fetch(ctx context.Context) (*Snapshot, error)
}

// CachedCatalog keeps the last Reference Data snapshot for ttl. Refreshes run
// one at a time; when a refresh fails the previous snapshot keeps being served.
type CachedCatalog struct {
	source snapshotFetcher
	ttl    time.Duration
	now    func() time.Time

	mu        sync.Mutex
	snapshot  *Snapshot
	fetchedAt time.Time
}

func NewCachedCatalog(source snapshotFetcher, ttl time.Duration) *CachedCatalog {
	return &CachedCatalog{source: source, ttl: ttl, now: time.Now}
}

func (c *CachedCatalog) Snapshot(ctx context.Context) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapshot != nil && c.now().Sub(c.fetchedAt) < c.ttl {
		return c.snapshot, nil
	}

	fresh, err := c.source.Fetch(ctx)
	if err != nil {
		if c.snapshot != nil {
			log.Printf("vision catalog: refresh failed, serving stale reference data: %v", err)
			// Wait another TTL before retrying so an outage isn't hammered on every request.
			c.fetchedAt = c.now()
			return c.snapshot, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}

	c.snapshot = fresh
	c.fetchedAt = c.now()
	return fresh, nil
}
