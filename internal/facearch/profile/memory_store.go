package profile

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-process Store for tests. It is not wired into the
// server: core-engine serves profiles from Postgres only (GormStore).
type MemoryStore struct {
	mu   sync.Mutex
	rows []FaceArchitectureProfile
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Put stores a row as given (tests use it to seed inactive or competing rows).
func (s *MemoryStore) Put(p FaceArchitectureProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	p.Status = NormalizeStatus(p.Status)
	s.rows = append(s.rows, p)
}

func (s *MemoryStore) Active(_ context.Context, brandID, appID string) (*FaceArchitectureProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sc := range Scopes(brandID, appID) {
		var best *FaceArchitectureProfile
		for i := range s.rows {
			r := &s.rows[i]
			if r.BrandID != sc[0] || r.ApplicationID != sc[1] || NormalizeStatus(r.Status) != StatusActive {
				continue
			}
			if best == nil || r.Version > best.Version || (r.Version == best.Version && r.CreatedAt.After(best.CreatedAt)) {
				best = r
			}
		}
		if best != nil {
			out := *best
			return &out, nil
		}
	}
	return nil, ErrNoActiveProfile
}

func (s *MemoryStore) Upsert(_ context.Context, in UpsertInput) (*FaceArchitectureProfile, error) {
	in, err := normaliseInput(in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := 1
	now := time.Now()
	for i := range s.rows {
		r := &s.rows[i]
		if r.BrandID == in.BrandID && r.ApplicationID == in.ApplicationID {
			if r.Code == in.Code && r.Version >= next {
				next = r.Version + 1
			}
			if NormalizeStatus(r.Status) == StatusActive {
				r.Status = StatusInactive
				r.UpdatedAt = &now
			}
		}
	}
	p := FaceArchitectureProfile{
		ID: uuid.NewString(), BrandID: in.BrandID, ApplicationID: in.ApplicationID, Code: in.Code,
		Title: in.Title, Status: StatusActive, Version: next, Document: string(in.Document),
		CreatedAt: now, UpdatedAt: &now,
	}
	s.rows = append(s.rows, p)
	return &p, nil
}
