package measureclient_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// recordedAnchors is testdata/anchors_fa-measure-1.json, written from the
// ai-worker's anchors.py by testdata/record_anchors.py. Regenerate it from the
// repository root with:
//
//	python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
type recordedAnchors struct {
	Source                 string           `json:"source"`
	Sha256                 string           `json:"sha256"`
	CatalogueVersion       string           `json:"catalogueVersion"`
	CatalogueVersionSource string           `json:"catalogueVersionSource"`
	FrameSource            string           `json:"frameSource"`
	FrameSha256            string           `json:"frameSha256"`
	NLandmarks             int              `json:"nLandmarks"`
	EyeCorners             map[string]int   `json:"eyeCorners"`
	Regions                map[string][]int `json:"regions"`
}

const regenCmd = "python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json"

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
	assert.Equal(t, r.CatalogueVersion, mc.RegionsCatalogueVersion)
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
	assertSourceHash(t, r.Source, r.Sha256)
}

// The canonical frame's definition (frame.py) is pinned the same way:
// guidance rebuilds that frame in Go (guidance.NewFrame).
func TestRecordingMatchesAiWorkerFrameSource(t *testing.T) {
	r := loadRecordedAnchors(t)
	require.Equal(t, "apps/workers/face/worker_face/facegeom/frame.py", r.FrameSource)
	assertSourceHash(t, r.FrameSource, r.FrameSha256)
}

// The recorded catalogue version is the one in this checkout's measures.py,
// parsed from the text as the recorder does (importing measures.py needs
// numpy and cv2).
func TestRecordingMatchesAiWorkerCatalogueVersion(t *testing.T) {
	r := loadRecordedAnchors(t)
	require.Equal(t, "apps/workers/face/worker_face/facegeom/measures.py", r.CatalogueVersionSource)
	m := regexp.MustCompile(`(?m)^CATALOGUE_VERSION = "([^"]+)"$`).FindAllSubmatch(readSource(t, r.CatalogueVersionSource), -1)
	require.Len(t, m, 1, "want exactly one CATALOGUE_VERSION assignment in %s", r.CatalogueVersionSource)
	assert.Equal(t, r.CatalogueVersion, string(m[0][1]),
		"%s changed catalogue version since the recording: regenerate it from the repository root with `%s` and re-check anchors.go", r.CatalogueVersionSource, regenCmd)
}

// readSource reads a repository file with line endings normalised to LF, as
// the recorder does, so a Windows checkout hashes the same.
func readSource(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile("../../../../../" + path)
	require.NoError(t, err, "the face worker source %s must be in this checkout (same repository)", path)
	return bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
}

func assertSourceHash(t *testing.T, path, want string) {
	t.Helper()
	sum := sha256.Sum256(readSource(t, path))
	assert.Equal(t, want, hex.EncodeToString(sum[:]),
		"%s changed since testdata/anchors_fa-measure-1.json was recorded: regenerate it from the repository root with `%s` and update anchors.go", path, regenCmd)
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
