package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dimensionsBody = `{"data":[
  {"code":"gut_health","name":"Gut Health"},
  {"code":"acne","name":"Acne & Blemishes","description":"Breakouts"},
  {"code":"pigmentation","name":"Pigmentation"}
],"success":true}`

const skinConditionsBody = `{"data":[
  {"code":"concern_melasma","name":"Melasma","dimensionCode":"pigmentation","visionCapabilities":["hyperpigmentation_net"]},
  {"code":"concern_acne_papule","name":"Acne Papule","dimensionCode":"acne","visionCapabilities":[" acne_lesion_classifier ","acne_lesion_classifier",""]},
  {"code":"concern_stress","name":"Stress","dimensionCode":"gut_health","visionCapabilities":[]},
  {"code":"concern_orphan","name":"Orphan","dimensionCode":"missing_dimension","visionCapabilities":["orphan_net"]}
],"success":true}`

func newReferenceServer(t *testing.T, dimensions, skinConditions string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		switch r.URL.Path {
		case "/api/reference/dimensions":
			_, _ = w.Write([]byte(dimensions))
		case "/api/reference/skin-conditions":
			_, _ = w.Write([]byte(skinConditions))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestReferenceClient_FetchKeepsOnlyVisionRelevantEntries(t *testing.T) {
	server := newReferenceServer(t, dimensionsBody, skinConditionsBody, http.StatusOK)

	snapshot, err := NewReferenceClient(server.URL+"/", 5*time.Second).Fetch(context.Background())
	require.NoError(t, err)

	require.Len(t, snapshot.Dimensions, 2)
	assert.Equal(t, "acne", snapshot.Dimensions[0].Code)
	assert.Equal(t, "Acne & Blemishes", snapshot.Dimensions[0].Name)
	assert.Equal(t, "Breakouts", snapshot.Dimensions[0].Description)
	require.Len(t, snapshot.Dimensions[0].Conditions, 1)
	assert.Equal(t, []string{"acne_lesion_classifier"}, snapshot.Dimensions[0].Conditions[0].Capabilities)
	assert.Equal(t, "pigmentation", snapshot.Dimensions[1].Code)

	var codes []string
	for _, c := range snapshot.Conditions {
		codes = append(codes, c.Code)
	}
	assert.Equal(t, []string{"concern_melasma", "concern_acne_papule", "concern_orphan"}, codes)
}

func TestReferenceClient_NonSuccessStatusIsError(t *testing.T) {
	server := newReferenceServer(t, dimensionsBody, skinConditionsBody, http.StatusInternalServerError)

	_, err := NewReferenceClient(server.URL, 5*time.Second).Fetch(context.Background())
	assert.Error(t, err)
}

func TestReferenceClient_MissingDataEnvelopeIsError(t *testing.T) {
	server := newReferenceServer(t, `{"success":true}`, skinConditionsBody, http.StatusOK)

	_, err := NewReferenceClient(server.URL, 5*time.Second).Fetch(context.Background())
	assert.Error(t, err)
}

type fakeFetcher struct {
	snapshot *Snapshot
	err      error
	calls    int
}

func (f *fakeFetcher) Fetch(ctx context.Context) (*Snapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.snapshot, nil
}

func newTestCache(source *fakeFetcher) (*CachedCatalog, *time.Time) {
	current := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	cache := NewCachedCatalog(source, time.Minute)
	cache.now = func() time.Time { return current }
	return cache, &current
}

func TestCachedCatalog_ReusesSnapshotWithinTTL(t *testing.T) {
	source := &fakeFetcher{snapshot: &Snapshot{Conditions: []Condition{{Code: "a"}}}}
	cache, clock := newTestCache(source)

	_, err := cache.Snapshot(context.Background())
	require.NoError(t, err)
	*clock = clock.Add(59 * time.Second)
	_, err = cache.Snapshot(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, source.calls)
}

func TestCachedCatalog_RefetchesAfterTTL(t *testing.T) {
	source := &fakeFetcher{snapshot: &Snapshot{Conditions: []Condition{{Code: "old"}}}}
	cache, clock := newTestCache(source)
	_, _ = cache.Snapshot(context.Background())

	source.snapshot = &Snapshot{Conditions: []Condition{{Code: "new"}}}
	*clock = clock.Add(61 * time.Second)
	snapshot, err := cache.Snapshot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, source.calls)
	assert.Equal(t, "new", snapshot.Conditions[0].Code)
}

func TestCachedCatalog_ServesStaleSnapshotWhenRefreshFails(t *testing.T) {
	source := &fakeFetcher{snapshot: &Snapshot{Conditions: []Condition{{Code: "old"}}}}
	cache, clock := newTestCache(source)
	_, _ = cache.Snapshot(context.Background())

	source.err = errors.New("connection refused")
	*clock = clock.Add(2 * time.Minute)
	snapshot, err := cache.Snapshot(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "old", snapshot.Conditions[0].Code)
}

func TestCachedCatalog_ColdFailureIsErrCatalogUnavailable(t *testing.T) {
	cache, _ := newTestCache(&fakeFetcher{err: errors.New("connection refused")})

	_, err := cache.Snapshot(context.Background())

	assert.True(t, errors.Is(err, ErrCatalogUnavailable))
}
