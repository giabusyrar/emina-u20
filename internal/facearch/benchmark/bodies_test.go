package benchmark_test

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// testModel stands in for the ai-worker manifest's sha256 in tests.
var testModel = strings.Repeat("ab", 32)

// measureBody is a synthetic /face-measure 200 body as JSON text, decoded
// by the harness exactly like a real one. A float64 value is an observed
// measurement without a band, a [3]float64 is {value, band low, band
// high}, and the string "occluded" is an occluded measurement.
func measureBody(values map[string]any) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var ms []string
	for _, k := range keys {
		switch v := values[k].(type) {
		case float64:
			ms = append(ms, fmt.Sprintf(`{"key":%q,"value":%s,"unit":"ratio","band":null,"visibility":"observed","proxy":false,"landmarks":[],"reason":null}`,
				k, strconv.FormatFloat(v, 'g', -1, 64)))
		case [3]float64:
			f := func(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }
			ms = append(ms, fmt.Sprintf(`{"key":%q,"value":%s,"unit":"ratio","band":[%s,%s],"visibility":"observed","proxy":false,"landmarks":[],"reason":null}`,
				k, f(v[0]), f(v[1]), f(v[2])))
		case string:
			ms = append(ms, fmt.Sprintf(`{"key":%q,"value":null,"unit":"ratio","band":null,"visibility":%q,"proxy":false,"landmarks":[],"reason":"requires region forehead"}`, k, v))
		}
	}
	return []byte(fmt.Sprintf(`{"catalogueVersion":"fa-measure/1","model":{"name":"face_landmarker","version":"test","manifestSha256":%q},`+
		`"quality":{"rollDeg":0,"yawDeg":0,"pitchDeg":0,"iodPx":100,"gatesPassed":["roll","yaw","pitch","iod_px"],"warnings":["perspective_not_corrected"],"regionDeltaE":{}},`+
		`"regions":{},"measurements":[%s],"landmarks":null}`, testModel, strings.Join(ms, ",")))
}

// Worker error bodies, shaped like the ai-worker's (docs/FACE-MEASURE.md).
var (
	gateBody      = []byte(`{"detail":{"code":"quality_gates_failed","failedGates":[{"gate":"yaw","value":30,"limit":15,"passed":false}],"quality":{}}}`)
	faceCountBody = []byte(`{"detail":{"code":"face_count","faces":0}}`)
	noModelBody   = []byte(`{"detail":{"code":"face_landmarker_unavailable","reason":"no model"}}`)
)
