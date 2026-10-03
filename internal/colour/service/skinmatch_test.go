package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/colour/domain"
)

// skinMatchSvc: the fake worker measures skin Lab {74.8, 8.55, 11.31}
// (V 7.41, C 2.84). Both foundation shades share the person's V/C so the
// quadrant rules keep them; one has exactly the skin's Lab, one is further.
func skinMatchSvc(t *testing.T, maxDeltaE *float64) *Service {
	t.Helper()
	cfg, err := domain.LoadConfig(filepath.Join("..", "..", "..", "config", "wcpa.json"))
	require.NoError(t, err)
	cfg.SkinMatch.MaxDeltaE00 = maxDeltaE
	store := fakeStore{c: []Candidate{
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "fdn-exact", ProductID: "f", Category: "complexion", Lab: domain.Lab{74.8, 8.55, 11.31}, V: 7.41, C: 2.84}},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "fdn-far", ProductID: "f", Category: "complexion", Lab: domain.Lab{68, 12, 20}, V: 7.41, C: 2.84}},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "lip1", ProductID: "l", Category: "lip", Lab: domain.Lab{74.8, 8.55, 11.31}, V: 7.41, C: 2.84}},
	}}
	return New(cfg, store, &fakeWorker{}, false)
}

func byID(recs []RecommendationOut) map[string]RecommendationOut {
	out := map[string]RecommendationOut{}
	for _, r := range recs {
		out[r.ShadeID] = r
	}
	return out
}

func TestSkinMatch_FoundationGetsDeltaEAndBand(t *testing.T) {
	out, err := skinMatchSvc(t, nil).Analyze(context.Background(), []byte("x"), "a.jpg", false, true, false)
	require.NoError(t, err)
	fdn := byID(out.Recommendations["complexion"])
	require.Contains(t, fdn, "fdn-exact")
	require.Contains(t, fdn, "fdn-far")

	exact := fdn["fdn-exact"]
	require.NotNil(t, exact.SkinDeltaE00)
	assert.Equal(t, 0.0, *exact.SkinDeltaE00)
	assert.Equal(t, domain.BandNotPerceptible, exact.SkinDeltaEBand)

	far := fdn["fdn-far"]
	require.NotNil(t, far.SkinDeltaE00)
	want := domain.DeltaE2000(domain.Lab{74.8, 8.55, 11.31}, domain.Lab{68, 12, 20})
	assert.InDelta(t, want, *far.SkinDeltaE00, 0.006)
	assert.Equal(t, domain.PerceptibilityBand(*far.SkinDeltaE00), far.SkinDeltaEBand)

	assert.Nil(t, exact.WithinTolerance, "no brand tolerance configured: no verdict")
}

func TestSkinMatch_OtherCategoriesGetNoSkinDifference(t *testing.T) {
	out, err := skinMatchSvc(t, nil).Analyze(context.Background(), []byte("x"), "a.jpg", false, true, false)
	require.NoError(t, err)
	for _, r := range out.Recommendations["lip"] {
		assert.Nil(t, r.SkinDeltaE00, "a lipstick is meant to differ from skin")
		assert.Empty(t, r.SkinDeltaEBand)
	}
}

func TestSkinMatch_BrandToleranceGivesAVerdict(t *testing.T) {
	limit := 1.0
	out, err := skinMatchSvc(t, &limit).Analyze(context.Background(), []byte("x"), "a.jpg", false, true, false)
	require.NoError(t, err)
	fdn := byID(out.Recommendations["complexion"])
	require.NotNil(t, fdn["fdn-exact"].WithinTolerance)
	assert.True(t, *fdn["fdn-exact"].WithinTolerance)
	require.NotNil(t, fdn["fdn-far"].WithinTolerance)
	assert.False(t, *fdn["fdn-far"].WithinTolerance)
}
