// Package facearch is the face-architecture interpretation layer: it asks
// the ai-worker for measurements and applies a brand's profile to them.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md.
package facearch

import (
	"fmt"

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
	Guidance   *guidance.Result `json:"guidance"`
	Provenance Provenance       `json:"provenance"`
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
		if reason := drift(doc, m, idx, spec.Keys()); reason != "" {
			out.Classifications[spec.Key] = classify.Unavailable(reason)
			continue
		}
		out.Classifications[spec.Key] = classify.NewPrototype(spec).Classify(idx)
	}
	for _, spec := range doc.Traits {
		if reason := drift(doc, m, idx, spec.Keys()); reason != "" {
			out.Traits[spec.Key] = traits.Unavailable(reason)
			continue
		}
		out.Traits[spec.Key] = traits.Evaluate(spec, idx)
	}
	if doc.Guidance != nil {
		out.Guidance = guidance.Resolve(doc.Guidance,
			guidance.Evidence{Classifications: out.Classifications, Traits: out.Traits},
			m.Landmarks, m.Regions)
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
