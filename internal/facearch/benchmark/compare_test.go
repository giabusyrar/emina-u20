package benchmark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// cmpResponse builds a measured worker response from scalar and list values.
// The image ids used with it ("secret-subject-...") must never reach a report.
func cmpResponse(scalars map[string]float64, lists map[string][]float64, occluded ...string) *measureclient.Response {
	r := &measureclient.Response{CatalogueVersion: "fa-measure/1"}
	for k, v := range scalars {
		v := v
		r.Measurements = append(r.Measurements, measureclient.Measurement{Key: k, Value: measureclient.Value{Scalar: &v}, Visibility: measureclient.VisibilityObserved})
	}
	for k, v := range lists {
		r.Measurements = append(r.Measurements, measureclient.Measurement{Key: k, Value: measureclient.Value{List: v}, Visibility: measureclient.VisibilityObserved})
	}
	for _, k := range occluded {
		r.Measurements = append(r.Measurements, measureclient.Measurement{Key: k, Visibility: measureclient.VisibilityOccluded})
	}
	return r
}

func cmpItem(id string, r *measureclient.Response) benchmark.Item {
	outcome := benchmark.OutcomeMeasured
	if r == nil {
		outcome = benchmark.OutcomeGateRejected
	}
	return benchmark.Item{Row: benchmark.ManifestRow{ImageID: id}, Outcome: outcome, Response: r}
}

func cmpRow(id, field, value string) benchmark.ResultRow {
	return benchmark.ResultRow{ImageID: id, Source: "makeover", Attribute: benchmark.RatioPrefix + field, Value: value}
}

func cmpMap(t *testing.T) *benchmark.MeasureMap {
	t.Helper()
	m, err := benchmark.ReadMeasureMap(mmWrite(t, mmValid))
	require.NoError(t, err)
	return m
}

func cmpFind(t *testing.T, r *benchmark.MeasureReport, name string) benchmark.ComparisonReport {
	t.Helper()
	for _, c := range r.Comparisons {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no comparison %q", name)
	return benchmark.ComparisonReport{}
}

func TestCompareInverseAgainstPairs(t *testing.T) {
	// width_length_ratio 0.5 and 0.8 -> ours on the vendor's scale 2.0 and 1.25.
	items := []benchmark.Item{
		cmpItem("secret-subject-a", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil)),
		cmpItem("secret-subject-b", cmpResponse(map[string]float64{"width_length_ratio": 0.8}, nil)),
	}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-a", "face_aspect_ratio", "1:1.9"),
		cmpRow("secret-subject-b", "face_aspect_ratio", "1.35"),
	}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue(), ModelSHA256: "m"})
	c := cmpFind(t, rep, "faceAspect")
	require.Len(t, c.Components, 1)
	a := c.Components[0]
	assert.Equal(t, 2, a.N)
	assert.InDelta(t, ((2.0-1.9)+(1.25-1.35))/2, *a.Bias, 1e-12)
	assert.Equal(t, 2, rep.Images)
}

func TestCompareAveragesLeftAndRightAndNeedsBoth(t *testing.T) {
	items := []benchmark.Item{
		cmpItem("secret-subject-a", cmpResponse(map[string]float64{"eye_aperture_ratio": 0.25}, nil)),
		cmpItem("secret-subject-b", cmpResponse(map[string]float64{"eye_aperture_ratio": 0.25}, nil)),
	}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-a", "left_eye_aspect_ratio", "1:3"),
		cmpRow("secret-subject-a", "right_eye_aspect_ratio", "1:5"),
		cmpRow("secret-subject-b", "left_eye_aspect_ratio", "1:3"), // right missing
	}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	c := cmpFind(t, rep, "eyeAspect")
	assert.Equal(t, 1, c.Components[0].N)
	assert.InDelta(t, 4.0-4.0, *c.Components[0].Bias, 1e-12) // ours 1/0.25 = 4, theirs mean(3, 5) = 4
	assert.Equal(t, 1, c.Excluded[benchmark.ExcludeVendorAbsent])
}

func TestComparePercentAndSharePercent(t *testing.T) {
	r := cmpResponse(map[string]float64{"thirds_upper_proxy": 1, "thirds_mid": 1, "thirds_lower": 2},
		map[string][]float64{"fifths": {0.1, 0.2, 0.4, 0.2, 0.1}})
	items := []benchmark.Item{cmpItem("secret-subject-a", r)}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-a", "horizontal_third", "25/25/50"),
		cmpRow("secret-subject-a", "vertical_fifth", "10/20/40/20/10"),
	}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	for _, name := range []string{"thirds", "fifths"} {
		c := cmpFind(t, rep, name)
		for i, a := range c.Components {
			require.Equal(t, 1, a.N, "%s[%d]", name, i)
			assert.InDelta(t, 0.0, *a.Bias, 1e-9, "%s[%d]", name, i)
		}
	}
	assert.Len(t, cmpFind(t, rep, "thirds").Components, 3)
	assert.Len(t, cmpFind(t, rep, "fifths").Components, 5)
}

func TestCompareCountsEveryExclusionReason(t *testing.T) {
	items := []benchmark.Item{
		cmpItem("secret-subject-rejected", nil),
		cmpItem("secret-subject-novendor", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil)),
		cmpItem("secret-subject-garbled", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil)),
		cmpItem("secret-subject-stale", cmpResponse(map[string]float64{}, nil)),
		cmpItem("secret-subject-occluded", cmpResponse(nil, nil, "width_length_ratio")),
		cmpItem("secret-subject-zero", cmpResponse(map[string]float64{"width_length_ratio": 0}, nil)),
	}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-rejected", "face_aspect_ratio", "1:1.4"),
		cmpRow("secret-subject-garbled", "face_aspect_ratio", "1,46"),
		cmpRow("secret-subject-stale", "face_aspect_ratio", "1:1.4"),
		cmpRow("secret-subject-occluded", "face_aspect_ratio", "1:1.4"),
		cmpRow("secret-subject-zero", "face_aspect_ratio", "1:1.4"),
		{ImageID: "secret-subject-novendor", Source: "wardah", Attribute: "ratio:face_aspect_ratio", Value: "1:1.4"}, // another source
	}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	c := cmpFind(t, rep, "faceAspect")
	assert.Equal(t, map[string]int{
		benchmark.ExcludeNotMeasured:       1,
		benchmark.ExcludeVendorAbsent:      1,
		benchmark.ExcludeVendorUnparseable: 1,
		benchmark.ExcludeOursAbsent:        1,
		benchmark.ExcludeOursNotUsable:     2,
	}, c.Excluded)
	assert.Equal(t, 0, c.Components[0].N)
}

func TestCompareReportsAKeyMissingFromTheCachedCatalogue(t *testing.T) {
	m := cmpMap(t)
	m.Comparisons[0].Ours = []string{"key_added_after_this_cache"}
	items := []benchmark.Item{cmpItem("secret-subject-a", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil))}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Map: m, Catalogue: facearchtest.Catalogue()})
	c := cmpFind(t, rep, "faceAspect")
	assert.Equal(t, map[string]int{benchmark.ExcludeNotInCatalogue: 1}, c.Excluded)
	assert.Equal(t, 0, c.Components[0].N)
}

func TestMeasureReportFilesCarryNoImageIDsAndAreDeterministic(t *testing.T) {
	var items []benchmark.Item
	var rows []benchmark.ResultRow
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("secret-subject-%d", i)
		items = append(items, cmpItem(id, cmpResponse(map[string]float64{"width_length_ratio": 0.6 + float64(i)/100}, nil)))
		rows = append(rows, cmpRow(id, "face_aspect_ratio", fmt.Sprintf("1:%.2f", 1.6+float64(i)/50)))
	}
	in := benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue(), ModelSHA256: "m"}
	dir1, dir2 := t.TempDir(), t.TempDir()
	require.NoError(t, benchmark.WriteMeasureReport(dir1, benchmark.CompareMeasurements(in)))
	require.NoError(t, benchmark.WriteMeasureReport(dir2, benchmark.CompareMeasurements(in)))
	for _, name := range []string{benchmark.MeasureReportJSONName, benchmark.MeasureReportMarkdownName} {
		a, err := os.ReadFile(filepath.Join(dir1, name))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(dir2, name))
		require.NoError(t, err)
		assert.Equal(t, string(a), string(b), name)
		assert.NotContains(t, string(a), "secret-subject", name)
	}
	var back benchmark.MeasureReport
	raw, err := os.ReadFile(filepath.Join(dir1, benchmark.MeasureReportJSONName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, 5, cmpFind(t, &back, "faceAspect").Components[0].N)
	md, err := os.ReadFile(filepath.Join(dir1, benchmark.MeasureReportMarkdownName))
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(md), "no pass/fail"), "the report says it does not judge")
}

func TestCompareUnknownTransformPanicsNamingCheck(t *testing.T) {
	m := cmpMap(t)
	m.Comparisons[0].Transform = "log"
	items := []benchmark.Item{cmpItem("secret-subject-a", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil))}
	rows := []benchmark.ResultRow{cmpRow("secret-subject-a", "face_aspect_ratio", "1:1.9")}
	assert.PanicsWithValue(t, `benchmark.CompareMeasurements: unknown transform "log" (call MeasureMap.Check first)`, func() {
		benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: m, Catalogue: facearchtest.Catalogue()})
	})
}

func TestCompareCountsAddUpToImages(t *testing.T) {
	good := cmpResponse(map[string]float64{"width_length_ratio": 0.5, "eye_aperture_ratio": 0.25, "thirds_upper_proxy": 1, "thirds_mid": 1, "thirds_lower": 2},
		map[string][]float64{"fifths": {0.1, 0.2, 0.4, 0.2, 0.1}})
	items := []benchmark.Item{
		cmpItem("secret-subject-a", good),
		cmpItem("secret-subject-b", good),
		cmpItem("secret-subject-c", nil),
		cmpItem("secret-subject-d", cmpResponse(nil, nil, "width_length_ratio")),
		cmpItem("secret-subject-e", cmpResponse(map[string]float64{}, nil)),
	}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-a", "face_aspect_ratio", "1:1.9"),
		cmpRow("secret-subject-b", "face_aspect_ratio", "1,9"),
		cmpRow("secret-subject-d", "face_aspect_ratio", "1:1.9"),
		cmpRow("secret-subject-e", "face_aspect_ratio", "1:1.9"),
		cmpRow("secret-subject-a", "left_eye_aspect_ratio", "1:3"),
		cmpRow("secret-subject-a", "right_eye_aspect_ratio", "1:5"),
		cmpRow("secret-subject-a", "horizontal_third", "25/25/50"),
		cmpRow("secret-subject-a", "vertical_fifth", "10/20/40/20/10"),
	}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	require.Len(t, rep.Comparisons, 4)
	for _, c := range rep.Comparisons {
		var excluded int
		for _, n := range c.Excluded {
			excluded += n
		}
		for k, a := range c.Components {
			assert.Equal(t, rep.Images, a.N+excluded, "%s[%d]", c.Name, k)
		}
	}
}

func TestParseVendorValueRejectsAnOverflowingQuotient(t *testing.T) {
	_, ok := benchmark.ParseVendorValue("1e-309:2", 1)
	assert.False(t, ok, "2 / 1e-309 is +Inf")
}

func TestCompareNeverPassesANonFiniteVendorValueToAgree(t *testing.T) {
	items := []benchmark.Item{
		cmpItem("secret-subject-a", cmpResponse(map[string]float64{"width_length_ratio": 0.5, "eye_aperture_ratio": 0.25}, nil)),
	}
	rows := []benchmark.ResultRow{
		cmpRow("secret-subject-a", "face_aspect_ratio", "1e-309:2"),
		cmpRow("secret-subject-a", "left_eye_aspect_ratio", "1e308"),
		cmpRow("secret-subject-a", "right_eye_aspect_ratio", "1e308"),
	}
	var rep *benchmark.MeasureReport
	require.NotPanics(t, func() {
		rep = benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	})
	assert.Equal(t, map[string]int{benchmark.ExcludeVendorUnparseable: 1}, cmpFind(t, rep, "faceAspect").Excluded)
	eye := cmpFind(t, rep, "eyeAspect")
	assert.Equal(t, 1, eye.Components[0].N+eye.Excluded[benchmark.ExcludeVendorUnparseable])
}

func TestMeasureMarkdownSaysWhenThereAreNoValuePairs(t *testing.T) {
	items := []benchmark.Item{cmpItem("secret-subject-a", cmpResponse(map[string]float64{"width_length_ratio": 0.5}, nil))}
	rep := benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	dir := t.TempDir()
	require.NoError(t, benchmark.WriteMeasureReport(dir, rep))
	md, err := os.ReadFile(filepath.Join(dir, benchmark.MeasureReportMarkdownName))
	require.NoError(t, err)
	assert.Contains(t, string(md), "No value pairs in any comparison")
	assert.Contains(t, string(md), "vendor_absent 1")

	rows := []benchmark.ResultRow{cmpRow("secret-subject-a", "face_aspect_ratio", "1:1.9")}
	rep = benchmark.CompareMeasurements(benchmark.CompareInput{Items: items, Results: rows, Map: cmpMap(t), Catalogue: facearchtest.Catalogue()})
	require.NoError(t, benchmark.WriteMeasureReport(dir, rep))
	md, err = os.ReadFile(filepath.Join(dir, benchmark.MeasureReportMarkdownName))
	require.NoError(t, err)
	assert.NotContains(t, string(md), "No value pairs")
}
