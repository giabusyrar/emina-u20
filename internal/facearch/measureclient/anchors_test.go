package measureclient_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// recordedAnchors is testdata/anchors_fa-measure-1.json, written from the
// ai-worker's anchors.py by the script in docs/superpowers/plans/
// 2026-09-29-face-architecture-guidance.md (Task 1). Regenerate it from the
// repository root with:
//
//	PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=apps/ai-worker python /tmp/record_anchors.py \
//	    > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
type recordedAnchors struct {
	Source     string           `json:"source"`
	Sha256     string           `json:"sha256"`
	NLandmarks int              `json:"nLandmarks"`
	EyeCorners map[string]int   `json:"eyeCorners"`
	Regions    map[string][]int `json:"regions"`
}

func loadRecordedAnchors(t *testing.T) recordedAnchors {
	t.Helper()
	raw, err := os.ReadFile("testdata/anchors_fa-measure-1.json")
	require.NoError(t, err)
	var r recordedAnchors
	require.NoError(t, json.Unmarshal(raw, &r))
	return r
}

// The Go table equals the recording, region for region, in order.
func TestAnchorTablesMatchRecording(t *testing.T) {
	r := loadRecordedAnchors(t)
	assert.Equal(t, r.NLandmarks, mc.NLandmarks)
	assert.Equal(t, map[string]int{"exoR": mc.ExoR, "endoR": mc.EndoR, "exoL": mc.ExoL, "endoL": mc.EndoL}, r.EyeCorners)
	names := make([]string, 0, len(r.Regions))
	for n := range r.Regions {
		names = append(names, n)
	}
	sort.Strings(names)
	assert.Equal(t, names, mc.RegionNames())
	for _, n := range names {
		assert.Equal(t, r.Regions[n], mc.RegionLandmarks(n), n)
	}
	assert.Contains(t, mc.RegionNames(), mc.RegionEyes)
}

// The recording is of the anchors.py in this checkout. Any change to that
// file (even a comment) fails here until the recording is regenerated and
// the Go table re-checked, so the two cannot drift silently. Line endings
// are normalised to LF, as the recorder does, so a Windows checkout hashes
// the same.
func TestRecordingMatchesAiWorkerAnchorsSource(t *testing.T) {
	r := loadRecordedAnchors(t)
	src, err := os.ReadFile("../../../../../" + r.Source)
	require.NoError(t, err, "the ai-worker source %s must be in this checkout (same repository)", r.Source)
	sum := sha256.Sum256(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")))
	assert.Equal(t, r.Sha256, hex.EncodeToString(sum[:]),
		"%s changed since testdata/anchors_fa-measure-1.json was recorded: regenerate it (see recordedAnchors) and update anchors.go", r.Source)
}

func TestRegionsOf(t *testing.T) {
	assert.Equal(t, []string{"jaw"}, mc.RegionsOf(172))
	assert.Equal(t, []string{"brows"}, mc.RegionsOf(9))
	assert.Equal(t, []string{"contour_right"}, mc.RegionsOf(234))
	assert.Empty(t, mc.RegionsOf(50), "a mid-cheek mesh point is in no region")
	assert.Empty(t, mc.RegionsOf(-1))
	assert.Nil(t, mc.RegionLandmarks("ears"))
}

func TestRegionLandmarksIsACopy(t *testing.T) {
	r := mc.RegionLandmarks("nose")
	r[0] = -1
	assert.Equal(t, 168, mc.RegionLandmarks("nose")[0])
}
