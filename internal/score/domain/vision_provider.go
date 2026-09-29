package domain

import (
	"context"
	"errors"
	"io"
)

// ErrVisionUnavailable means the configured vision provider failed or timed
// out. Surfaced as its own error, not a partial result, because an axis like
// W/T (Aging) has no meaning without a vision-side value.
var ErrVisionUnavailable = errors.New("vision provider unavailable")

// VisionSubmission is what a caller already has in hand before it can ask a
// vision provider anything: the applicant's personal data (consent already
// collected) and their photo.
type VisionSubmission struct {
	FullName              string
	PhoneNumber           string
	Email                 string
	DateOfBirth           string // YYYY-MM-DD
	ConsentDataProcessing bool
	ConsentMarketing      bool
	Image                 io.Reader
	ImageFilename         string
}

// VisionResult is a provider's response, reduced to what the evaluate
// pipeline needs: every numeric leaf value flattened by its full dot-path
// from the response root (e.g. "inference_result.results.skin_scoring.Pores")
// — matching how a ruleset's field_mapping.vision values are recorded — plus
// a trace id for support.
type VisionResult struct {
	Flattened     map[string]float64
	TransactionID string
}

// VisionProvider is one CV vendor/service capable of analysing a photo.
// Score Engine resolves which implementation to use for a given ruleset via
// the ruleset schema's own vision_source_code field, so a ruleset never
// hardcodes a vendor — only the provider registry (currently a small static
// map; a real catalog entity replaces it later) does.
type VisionProvider interface {
	// Code is this provider's vision_source_code, e.g. "paradev_skin_analyzer".
	Code() string
	Submit(ctx context.Context, sub VisionSubmission) (*VisionResult, error)
}

// VisionProviderRegistry resolves a ruleset's vision_source_code to the
// VisionProvider that should serve it. This first version is a small static
// map built at startup (Paradev is the only entry) — Fase 1 replaces the
// construction site with one backed by a real provider catalog entity,
// without any evaluate-path code needing to change, since callers only ever
// see Resolve.
type VisionProviderRegistry struct {
	providers map[string]VisionProvider
}

func NewVisionProviderRegistry(providers ...VisionProvider) *VisionProviderRegistry {
	m := make(map[string]VisionProvider, len(providers))
	for _, p := range providers {
		m[p.Code()] = p
	}
	return &VisionProviderRegistry{providers: m}
}

func (r *VisionProviderRegistry) Resolve(code string) (VisionProvider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.providers[code]
	return p, ok
}
