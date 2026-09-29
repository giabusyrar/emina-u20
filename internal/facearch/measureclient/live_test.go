package measureclient_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// liveTimeout bounds the opt-in call below; test-only.
const liveTimeout = 30 * time.Second

// TestRecordedCatalogueMatchesLiveWorker checks the recorded catalogue
// against a running ai-worker (Plan 1 landed). Opt-in: set
// FACEARCH_TEST_AI_WORKER_URL, e.g. http://127.0.0.1:8088.
func TestRecordedCatalogueMatchesLiveWorker(t *testing.T) {
	url := os.Getenv("FACEARCH_TEST_AI_WORKER_URL")
	if url == "" {
		t.Skip("FACEARCH_TEST_AI_WORKER_URL not set")
	}
	live, err := mc.New(url, liveTimeout).Catalogue(context.Background())
	require.NoError(t, err)
	rec := facearchtest.Catalogue()
	assert.Equal(t, rec.CatalogueVersion, live.CatalogueVersion)
	require.Equal(t, len(rec.Measurements), len(live.Measurements))
	for i, want := range rec.Measurements {
		got := live.Measurements[i]
		assert.Equal(t, want.Key, got.Key)
		assert.Equal(t, want.Unit, got.Unit, want.Key)
		assert.Equal(t, want.Regions, got.Regions, want.Key)
		assert.Equal(t, want.Shape, got.Shape, want.Key)
		assert.Equal(t, want.Landmarks, got.Landmarks, want.Key)
		assert.Equal(t, want.Supported, got.Supported, want.Key)
		assert.Equal(t, want.Proxy, got.Proxy, want.Key)
	}
}
