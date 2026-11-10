package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
)

type countingStore struct {
	lists  int
	gets   int
	err    error
	shades []Candidate
}

func (s *countingStore) ListCandidates(context.Context) ([]Candidate, error) {
	s.lists++
	if s.err != nil {
		return nil, s.err
	}
	return s.shades, nil
}

func (s *countingStore) GetCandidates(context.Context, []string) ([]Candidate, error) {
	s.gets++
	return s.shades, nil
}

func candidate(id string) Candidate {
	return Candidate{ShadeCandidate: domain.ShadeCandidate{ShadeID: id}}
}

func TestCachedStore_ServesFromCacheWithinTTL(t *testing.T) {
	store := &countingStore{shades: []Candidate{candidate("s1")}}
	clock := time.Unix(0, 0)
	c := NewCachedStore(store, time.Minute)
	c.now = func() time.Time { return clock }

	for i := 0; i < 3; i++ {
		got, err := c.ListCandidates(context.Background())
		require.NoError(t, err)
		assert.Len(t, got, 1)
	}
	assert.Equal(t, 1, store.lists)

	clock = clock.Add(time.Minute)
	_, _ = c.ListCandidates(context.Background())
	assert.Equal(t, 2, store.lists, "refreshed after the TTL")
}

func TestCachedStore_KeepsServingThePreviousListWhenARefreshFails(t *testing.T) {
	store := &countingStore{shades: []Candidate{candidate("s1")}}
	clock := time.Unix(0, 0)
	c := NewCachedStore(store, time.Minute)
	c.now = func() time.Time { return clock }
	_, _ = c.ListCandidates(context.Background())

	store.err = errors.New("db down")
	clock = clock.Add(2 * time.Minute)
	got, err := c.ListCandidates(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "s1", got[0].ShadeID)

	_, _ = c.ListCandidates(context.Background())
	assert.Equal(t, 2, store.lists, "a failed refresh waits another TTL before retrying")
}

func TestCachedStore_ColdFailureIsAnError(t *testing.T) {
	c := NewCachedStore(&countingStore{err: errors.New("db down")}, time.Minute)
	_, err := c.ListCandidates(context.Background())
	assert.Error(t, err)
}

func TestCachedStore_GetCandidatesIsNotCached(t *testing.T) {
	store := &countingStore{shades: []Candidate{candidate("s1")}}
	c := NewCachedStore(store, time.Minute)
	_, _ = c.GetCandidates(context.Background(), []string{"s1"})
	_, _ = c.GetCandidates(context.Background(), []string{"s1"})
	assert.Equal(t, 2, store.gets)
}
