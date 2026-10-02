package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// CachedStore keeps the shade list (ListCandidates) for ttl, so an analysis
// does not read the catalog from the database every time. Refreshes run one at
// a time; when a refresh fails the previous list keeps being served (the same
// behaviour as the vision reference-data cache). GetCandidates, a lookup by
// shade id for try-on, always goes to the store.
type CachedStore struct {
	store Store
	ttl   time.Duration
	now   func() time.Time

	mu        sync.Mutex
	shades    []Candidate
	fetchedAt time.Time
}

func NewCachedStore(store Store, ttl time.Duration) *CachedStore {
	return &CachedStore{store: store, ttl: ttl, now: time.Now}
}

func (c *CachedStore) ListCandidates(ctx context.Context) ([]Candidate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shades != nil && c.now().Sub(c.fetchedAt) < c.ttl {
		return c.shades, nil
	}
	fresh, err := c.store.ListCandidates(ctx)
	if err != nil {
		if c.shades != nil {
			log.Printf("colour catalog: refresh failed, serving the previous shade list: %v", err)
			// Wait another TTL before retrying so an outage isn't hit on every request.
			c.fetchedAt = c.now()
			return c.shades, nil
		}
		return nil, err
	}
	if fresh == nil {
		fresh = []Candidate{}
	}
	c.shades = fresh
	c.fetchedAt = c.now()
	return fresh, nil
}

func (c *CachedStore) GetCandidates(ctx context.Context, shadeIDs []string) ([]Candidate, error) {
	return c.store.GetCandidates(ctx, shadeIDs)
}
