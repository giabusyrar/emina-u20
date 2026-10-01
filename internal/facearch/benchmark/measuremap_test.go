package benchmark_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
)

// mmValid is a map that passes Check; every number in it is a test input.
const mmValid = `{
  "catalogueVersion": "fa-measure/1", "source": "makeover", "cite": "test",
  "comparisons": [
    {"name": "faceAspect", "vendor": ["face_aspect_ratio"], "components": 1,
     "ours": ["width_length_ratio"], "transform": "inverse", "note": "n"},
    {"name": "fifths", "vendor": ["vertical_fifth"], "components": 5,
     "ours": ["fifths"], "transform": "percent", "note": "n"},
    {"name": "thirds", "vendor": ["horizontal_third"], "components": 3,
     "ours": ["thirds_upper_proxy", "thirds_mid", "thirds_lower"], "transform": "sharePercent", "note": "n"},
    {"name": "eyeAspect", "vendor": ["left_eye_aspect_ratio", "right_eye_aspect_ratio"], "components": 1,
     "ours": ["eye_aperture_ratio"], "transform": "inverse", "note": "n"}
  ],
  "unused": {"overall_eye_height_to_eyebrow_distance": "redundant"}
}`

func mmWrite(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "measure-map.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestReadMeasureMapAcceptsAValidMap(t *testing.T) {
	m, err := benchmark.ReadMeasureMap(mmWrite(t, mmValid))
	require.NoError(t, err)
	assert.Equal(t, "makeover", m.Source)
	assert.Len(t, m.Comparisons, 4)
	assert.NoError(t, m.CheckShapes(facearchtest.Catalogue()))
}

func TestReadMeasureMapRefusesMalformedMaps(t *testing.T) {
	for name, edit := range map[string][2]string{
		"unknown field":        {`"cite": "test",`, `"cite": "test", "extra": 1,`},
		"unknown source":       {`"source": "makeover"`, `"source": "acme"`},
		"no catalogue":         {`"catalogueVersion": "fa-measure/1"`, `"catalogueVersion": ""`},
		"unknown transform":    {`"transform": "percent"`, `"transform": "log"`},
		"inverse of a list":    {`"ours": ["width_length_ratio"], "transform": "inverse"`, `"ours": ["width_length_ratio", "fifths"], "transform": "inverse"`},
		"percent of 1":         {`"components": 5`, `"components": 1`},
		"share count mismatch": {`"components": 3`, `"components": 2`},
		"empty note":           {`"note": "n"`, `"note": ""`},
		"duplicate name":       {`"name": "fifths"`, `"name": "faceAspect"`},
		"vendor field twice":   {`["vertical_fifth"]`, `["face_aspect_ratio"]`},
		"unused also compared": {`"overall_eye_height_to_eyebrow_distance"`, `"face_aspect_ratio"`},
		"no vendor":            {`["horizontal_third"]`, `[]`},
	} {
		require.Contains(t, mmValid, edit[0], name)
		_, err := benchmark.ReadMeasureMap(mmWrite(t, strings.Replace(mmValid, edit[0], edit[1], 1)))
		assert.Error(t, err, name)
	}
}

func TestCheckShapes(t *testing.T) {
	m, err := benchmark.ReadMeasureMap(mmWrite(t, mmValid))
	require.NoError(t, err)
	// A key the cached catalogue lacks is not an error here: the cache may
	// predate the key (CompareMeasurements reports it).
	m.Comparisons[0].Ours = []string{"not_in_any_catalogue"}
	assert.NoError(t, m.CheckShapes(facearchtest.Catalogue()))
	// A scalar transform on a list key, or a list transform on a scalar key, is an error.
	m.Comparisons[0].Ours = []string{"fifths"}
	assert.ErrorContains(t, m.CheckShapes(facearchtest.Catalogue()), "fifths")
	m.Comparisons[0].Ours = []string{"width_length_ratio"}
	m.Comparisons[1].Ours = []string{"width_length_ratio"}
	assert.ErrorContains(t, m.CheckShapes(facearchtest.Catalogue()), "width_length_ratio")
	// An unsupported key can never be compared.
	m.Comparisons[1].Ours = []string{"fifths"}
	m.Comparisons[0].Ours = []string{"eyelid_type"}
	assert.ErrorContains(t, m.CheckShapes(facearchtest.Catalogue()), "eyelid_type")
}

func TestParseVendorValue(t *testing.T) {
	ok := map[string]struct {
		raw  string
		n    int
		want []float64
	}{
		"pair":          {"1:1.46", 1, []float64{1.46}},
		"pair spaced":   {" 1 : 1.46 ", 1, []float64{1.46}},
		"pair not one":  {"2:3", 1, []float64{1.5}},
		"bare":          {"1.46", 1, []float64{1.46}},
		"thirds":        {"33/34/33", 3, []float64{33, 34, 33}},
		"fifths spaced": {"20 / 20 / 20 / 20 / 20", 5, []float64{20, 20, 20, 20, 20}},
	}
	for name, c := range ok {
		got, parsed := benchmark.ParseVendorValue(c.raw, c.n)
		require.True(t, parsed, name)
		assert.InDeltaSlice(t, c.want, got, 1e-12, name)
	}
	for name, c := range map[string]struct {
		raw string
		n   int
	}{
		"comma decimal":    {"1,46", 1},
		"comma pair":       {"1 : 1,46", 1},
		"zero first":       {"0:1.46", 1},
		"negative first":   {"-1:1.46", 1},
		"words":            {"wide", 1},
		"empty":            {"", 1},
		"too few":          {"33/33", 3},
		"too many":         {"20/20/20/20/20/20", 5},
		"pair for a list":  {"1:1.46", 3},
		"not finite":       {"NaN", 1},
		"infinite":         {"Inf", 1},
		"three-part ratio": {"1:2:3", 1},
	} {
		_, parsed := benchmark.ParseVendorValue(c.raw, c.n)
		assert.False(t, parsed, name)
	}
}
