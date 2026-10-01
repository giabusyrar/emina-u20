// Package facearch is the face-architecture interpretation layer: it asks
// the ai-worker for measurements and applies a brand's profile to them.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md.
package facearch

import (
	"fmt"
	"math"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/guidance"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// ProfileRef identifies the profile that produced a response.
type ProfileRef struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Version          int    `json:"version"`
	CatalogueVersion string `json:"catalogueVersion"`
}

// CalibrationRef repeats the profile's calibration status on every response.
type CalibrationRef struct {
	Status         string  `json:"status"`
	BenchmarkRunID *string `json:"benchmarkRunId,omitempty"`
	Target         *string `json:"target,omitempty"`
}

// Provenance says what produced the answer.
type Provenance struct {
	Model            measureclient.Model `json:"model"`
	CatalogueVersion string              `json:"catalogueVersion"`
	Profile          ProfileRef          `json:"profile"`
	Calibration      CalibrationRef      `json:"calibration"`
}

// Response is the body of POST /api/v1/vision/face-architecture/:brandId/:applicationId.
type Response struct {
	Quality measureclient.Quality `json:"quality"`
	Regions map[string]string     `json:"regions"`
	// Measurements are the profile's measurementsShown, as the ai-worker
	// returned them, in the worker's order.
	Measurements []measureclient.Measurement `json:"measurements"`
	// MeasurementsMissing lists measurementsShown keys the worker did not
	// return (catalogue drift); omitted when empty.
	MeasurementsMissing []string                   `json:"measurementsMissing,omitempty"`
	Classifications     map[string]classify.Result `json:"classifications"`
	Traits              map[string]traits.Result   `json:"traits"`
	// Guidance is the profile's makeup placement for this face (package
	// guidance); JSON null when the profile has no guidance section.
	Guidance *guidance.Result `json:"guidance"`
	// Landmarks are the worker's landmarks in image pixels, the space of
	// guidance polygons, indexed as the worker indexes them, for drawing
	// only: measurements stay in the canonical IOD frame. Each point is
	// [x, y] exactly as returned; any further component is dropped. JSON null
	// unless the worker returned all of them, each with finite x and y.
	Landmarks [][2]float64 `json:"landmarks"`
	// Landmarks3D are Landmarks with the worker's relative-depth estimate as
	// a third component, for 3D display only; never measured from. JSON null
	// unless the worker returned all of them, each with finite x, y and z.
	Landmarks3D [][3]float64 `json:"landmarks3d"`
	Provenance  Provenance   `json:"provenance"`
}

// displayLandmarks returns m's landmarks as [x, y] pixels, or nil when they
// are absent or unusable. Never a partial list: a drawing indexed by
// landmark number must not silently shift.
func displayLandmarks(pts [][]float64) [][2]float64 {
	if len(pts) != measureclient.NLandmarks {
		return nil
	}
	out := make([][2]float64, len(pts))
	for i, p := range pts {
		if len(p) < 2 || math.IsNaN(p[0]) || math.IsInf(p[0], 0) || math.IsNaN(p[1]) || math.IsInf(p[1], 0) {
			return nil
		}
		out[i] = [2]float64{p[0], p[1]}
	}
	return out
}

// displayLandmarks3D is displayLandmarks for [x, y, z]; nil, never partial.
func displayLandmarks3D(pts [][]float64) [][3]float64 {
	if len(pts) != measureclient.NLandmarks {
		return nil
	}
	out := make([][3]float64, len(pts))
	for i, p := range pts {
		if len(p) < 3 {
			return nil
		}
		for _, v := range p[:3] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil
			}
		}
		out[i] = [3]float64{p[0], p[1], p[2]}
	}
	return out
}

// Interpret applies a loaded profile document to one measure response.
// It never fails: drift between the profile and the worker's catalogue
// marks the affected classifier or trait "unavailable" with a reason.
func Interpret(p *profile.FaceArchitectureProfile, doc *profile.Document, m *measureclient.Response) *Response {
	idx := m.Index()
	out := &Response{
		Quality:         m.Quality,
		Regions:         m.Regions,
		Measurements:    []measureclient.Measurement{},
		Classifications: map[string]classify.Result{},
		Traits:          map[string]traits.Result{},
		Landmarks:       displayLandmarks(m.Landmarks),
		Landmarks3D:     displayLandmarks3D(m.Landmarks3D),
		Provenance: Provenance{
			Model:            m.Model,
			CatalogueVersion: m.CatalogueVersion,
			Profile:          ProfileRef{ID: p.ID, Code: p.Code, Version: p.Version, CatalogueVersion: doc.CatalogueVersion},
			Calibration: CalibrationRef{
				Status:         doc.Calibration.Status,
				BenchmarkRunID: doc.Calibration.BenchmarkRunID,
				Target:         doc.Calibration.Target,
			},
		},
	}

	shown := make(map[string]bool, len(doc.MeasurementsShown))
	for _, k := range doc.MeasurementsShown {
		shown[k] = true
	}
	returned := map[string]bool{}
	for _, meas := range m.Measurements {
		returned[meas.Key] = true
		if shown[meas.Key] {
			out.Measurements = append(out.Measurements, meas)
		}
	}
	for _, k := range doc.MeasurementsShown {
		if !returned[k] {
			out.MeasurementsMissing = append(out.MeasurementsMissing, k)
		}
	}

	for _, spec := range doc.Classifiers {
		keys := spec.Keys()
		var res classify.Result
		if reason := drift(doc, m, idx, keys); reason != "" {
			res = classify.Unavailable(reason)
		} else {
			res = classify.NewPrototype(spec).Classify(idx)
		}
		res.Measurements = keys
		out.Classifications[spec.Key] = res
	}
	for _, spec := range doc.Traits {
		keys := spec.Keys()
		var res traits.Result
		if reason := drift(doc, m, idx, keys); reason != "" {
			res = traits.Unavailable(reason)
		} else {
			res = traits.Evaluate(spec, idx)
		}
		res.Measurements = keys
		out.Traits[spec.Key] = res
	}
	if doc.Guidance != nil {
		out.Guidance = guidance.Resolve(doc.Guidance,
			guidance.Evidence{Classifications: out.Classifications, Traits: out.Traits},
			m.Landmarks, m.Regions, m.CatalogueVersion)
	}
	return out
}

// drift explains why keys cannot be used with this response, or returns "".
// Occlusion is not drift: an occluded key is present with visibility
// "occluded" and handled by the classifier or trait itself.
func drift(doc *profile.Document, m *measureclient.Response, idx measureclient.Index, keys []string) string {
	if doc.CatalogueVersion != m.CatalogueVersion {
		return fmt.Sprintf("catalogue drift: profile targets %s, ai-worker measured with %s", doc.CatalogueVersion, m.CatalogueVersion)
	}
	for _, k := range keys {
		switch idx.Observe(k).State {
		case measureclient.StateMissing:
			return fmt.Sprintf("catalogue drift: measurement %s is not in %s", k, m.CatalogueVersion)
		case measureclient.StateNotSupported:
			return fmt.Sprintf("catalogue drift: measurement %s is not supported in %s", k, m.CatalogueVersion)
		case measureclient.StateNotScalar:
			return fmt.Sprintf("catalogue drift: measurement %s is list-valued in %s", k, m.CatalogueVersion)
		}
	}
	return ""
}
