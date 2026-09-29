// Package measureclient calls the ai-worker's face-measure endpoints
// (vision profile) and mirrors their JSON. The contract is the ai-worker's
// ai_worker/vision/schemas/face_measure.py and routers/face_measure.py
// (docs/FACE-MEASURE.md). Field names, order and nullability here follow
// those Pydantic models exactly; do not add fields the worker does not send.
package measureclient

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Paths on the ai-worker (vision profile).
const (
	MeasurePath   = "/api/v1/face-measure"
	CataloguePath = "/api/v1/face-measure/catalogue"
)

// Visibility values a measurement can carry (Plan 1, pipeline.MeasurementResult).
const (
	VisibilityObserved     = "observed"
	VisibilityUnknown      = "unknown"
	VisibilityOccluded     = "occluded"
	VisibilityNotSupported = "not_supported"
)

// Value is a measurement value: a float, a list of floats, or null.
// The zero Value is null.
type Value struct {
	Scalar *float64
	List   []float64
}

// IsNull reports whether the value is JSON null.
func (v Value) IsNull() bool { return v.Scalar == nil && v.List == nil }

// IsList reports whether the value is a JSON array.
func (v Value) IsList() bool { return v.List != nil }

func (v Value) MarshalJSON() ([]byte, error) {
	switch {
	case v.List != nil:
		return json.Marshal(v.List)
	case v.Scalar != nil:
		return json.Marshal(*v.Scalar)
	default:
		return []byte("null"), nil
	}
}

func (v *Value) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*v = Value{}
	switch {
	case bytes.Equal(b, []byte("null")):
		return nil
	case len(b) > 0 && b[0] == '[':
		list := []float64{}
		if err := json.Unmarshal(b, &list); err != nil {
			return fmt.Errorf("measurement value list: %w", err)
		}
		v.List = list
		return nil
	default:
		var f float64
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("measurement value: %w", err)
		}
		v.Scalar = &f
		return nil
	}
}

// Measurement mirrors MeasurementOut. Value is null when the measurement is
// occluded or out of frame (visibility "occluded"; every measurement when
// the eyes are occluded, reason "canonical frame requires region eyes"),
// not supported, or undefined for this face (visibility "unknown", reason
// "degenerate geometry"). Band is [lo, hi], each a number or a list like
// Value, re-centred on the value so it always contains it; it is null
// whenever Value is, when bands are disabled, or when too few perturbed
// samples were finite.
type Measurement struct {
	Key        string    `json:"key"`
	Value      Value     `json:"value"`
	Unit       string    `json:"unit"`
	Band       *[2]Value `json:"band"`
	Visibility string    `json:"visibility"`
	Proxy      bool      `json:"proxy"`
	Landmarks  []int     `json:"landmarks"`
	Reason     *string   `json:"reason"`
}

// Model mirrors ModelOut.
type Model struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	ManifestSha256 string `json:"manifestSha256"`
}

// Quality mirrors QualityOut. RegionDeltaE omits regions that are out of
// frame. Warnings may include perspective_not_corrected (always),
// visibility_detector_unconfigured, visibility_reference_unavailable and
// face_partly_out_of_frame; core-engine passes them through unread.
type Quality struct {
	RollDeg      float64            `json:"rollDeg"`
	YawDeg       float64            `json:"yawDeg"`
	PitchDeg     float64            `json:"pitchDeg"`
	IodPx        float64            `json:"iodPx"`
	GatesPassed  []string           `json:"gatesPassed"`
	Warnings     []string           `json:"warnings"`
	RegionDeltaE map[string]float64 `json:"regionDeltaE"`
}

// Response mirrors FaceMeasureResponse. Landmarks is 478 x [x, y] in image
// pixels; core-engine always requests it (landmarks=true) for Plan 3's
// guidance polygons.
type Response struct {
	CatalogueVersion string            `json:"catalogueVersion"`
	Model            Model             `json:"model"`
	Quality          Quality           `json:"quality"`
	Regions          map[string]string `json:"regions"`
	Measurements     []Measurement     `json:"measurements"`
	Landmarks        [][]float64       `json:"landmarks"`
}

// CatalogueEntry mirrors CatalogueEntryOut. Shape is "scalar" (value is a
// number) or "list" (a list of numbers); see Catalogue.Shape.
type CatalogueEntry struct {
	Key       string   `json:"key"`
	Unit      string   `json:"unit"`
	Shape     string   `json:"shape"`
	Regions   []string `json:"regions"`
	Landmarks []int    `json:"landmarks"`
	Proxy     bool     `json:"proxy"`
	Supported bool     `json:"supported"`
	Reason    *string  `json:"reason"`
}

// Catalogue mirrors CatalogueOut.
type Catalogue struct {
	CatalogueVersion string           `json:"catalogueVersion"`
	Measurements     []CatalogueEntry `json:"measurements"`
}

// Entry returns the catalogue entry for key.
func (c *Catalogue) Entry(key string) (CatalogueEntry, bool) {
	for _, e := range c.Measurements {
		if e.Key == key {
			return e, true
		}
	}
	return CatalogueEntry{}, false
}
