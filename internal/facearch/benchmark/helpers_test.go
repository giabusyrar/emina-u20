package benchmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testDoc is a small uncalibrated profile over two scalar measurements.
// Its numbers are test inputs, not proposed values.
const testDoc = `{
  "schemaVersion": 1, "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test vocabulary", "for": "classifiers"},
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"},
      {"cite": "test", "for": "traits.noseWidth.rules"}
    ]},
  "measurementsShown": ["width_length_ratio"],
  "classifiers": [{"key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5, "classes": [
    {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
    {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}]}],
  "traits": [{"key": "noseWidth", "rules": [
    {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]},
    {"label": "Narrow", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": "<", "value": 0.9}]}],
    "default": "Average"}]
}`

// testLabelMap maps Perfect Corp and rater vocabularies onto testDoc.
const testLabelMap = `{"attributes": {
  "faceShape": {"ours": "faceShape", "values": {
    "perfectcorp": {"Oblong": "Long", "Round": "Wide", "Unknown": null},
    "rater": {"long": "Long", "wide": "Wide"}}},
  "noseWidth": {"ours": "noseWidth", "values": {
    "perfectcorp": {"Broad": "Broad", "Narrow": "Narrow", "Average": "Average"}}}
}}`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
