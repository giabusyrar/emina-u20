package guidance_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/guidance"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

// syntheticFace puts every landmark at (400, 300) except the eye corners,
// which make eye centres 200 px apart on a line rolled by rollDeg about
// (400, 300). Synthetic input with a known frame.
func syntheticFace(rollDeg float64) [][]float64 {
	l := make([][]float64, measureclient.NLandmarks)
	for i := range l {
		l[i] = []float64{400, 300}
	}
	r := rollDeg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	at := func(u float64) []float64 { return []float64{400 + u*c, 300 + u*s} }
	// Right eye centre at u = -100 (corners at -130 and -70), left at +100.
	l[measureclient.ExoR], l[measureclient.EndoR] = at(-130), at(-70)
	l[measureclient.EndoL], l[measureclient.ExoL] = at(70), at(130)
	return l
}

func TestFrame_KnownGeometry(t *testing.T) {
	for _, roll := range []float64{0, 7, -12} {
		l := syntheticFace(roll)
		f, err := guidance.NewFrame(l)
		require.NoError(t, err)
		assert.InDelta(t, 200, f.IodPx(), 1e-9)
		r := roll * math.Pi / 180
		// One IOD straight down the face (perpendicular to the eye line).
		below := [2]float64{400 - 200*math.Sin(r), 300 + 200*math.Cos(r)}
		c := f.ToCanonical(below)
		assert.InDelta(t, 0, c[0], 1e-9, "roll %v", roll)
		assert.InDelta(t, 1, c[1], 1e-9, "roll %v", roll)
		leftCentre := [2]float64{400 + 100*math.Cos(r), 300 + 100*math.Sin(r)}
		c = f.ToCanonical(leftCentre)
		assert.InDelta(t, 0.5, c[0], 1e-9, "+x is toward the subject's left (image right)")
		assert.InDelta(t, 0, c[1], 1e-9)
		p := [2]float64{123.4, 567.8}
		back := f.ToImage(f.ToCanonical(p))
		assert.InDelta(t, p[0], back[0], 1e-9)
		assert.InDelta(t, p[1], back[1], 1e-9)
	}
}

// The Go frame reproduces the face worker's frame.canonicalise on the same
// recorded landmarks. Expected values were printed by the worker on
// 2026-09-29 (repository root):
//
//	PYTHONPATH=apps/workers/face python -c "import json, numpy as np; from worker_face.facegeom.frame import canonicalise; d = json.load(open('internal/facearch/facearchtest/testdata/measure_fixture_face.json')); f = canonicalise(np.array(d['landmarks'], float)); print(repr(f.iod_px), [f.points[i].tolist() for i in (1, 152, 234, 454, 10)])"
func TestFrame_AgreesWithWorkerOnFixture(t *testing.T) {
	l := facearchtest.FixtureFace().Landmarks
	f, err := guidance.NewFrame(l)
	require.NoError(t, err)
	assert.InDelta(t, 228.22322679779995, f.IodPx(), 1e-9)
	want := map[int][2]float64{
		1:   {-0.01580165271881843, 0.594157922907696},
		152: {0.018685105234812413, 1.8204098863431526},
		234: {-1.0923884919493354, 0.2856101388205952},
		454: {1.14199773196521, 0.28003636976872304},
		10:  {-0.019282934688128913, -0.8017088665146592},
	}
	for i, w := range want {
		got := f.ToCanonical([2]float64{l[i][0], l[i][1]})
		assert.InDelta(t, w[0], got[0], 1e-9, "landmark %d x", i)
		assert.InDelta(t, w[1], got[1], 1e-9, "landmark %d y", i)
	}
}

func TestFrame_Errors(t *testing.T) {
	_, err := guidance.NewFrame(make([][]float64, 10))
	assert.ErrorContains(t, err, "want 478 landmarks, got 10")

	l := syntheticFace(0)
	l[5] = []float64{math.NaN(), 1}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "landmark 5 is not a finite [x, y]")

	l = syntheticFace(0)
	l[7] = []float64{1}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "landmark 7 is not a finite [x, y]")

	l = syntheticFace(0)
	for _, i := range []int{measureclient.ExoR, measureclient.EndoR, measureclient.ExoL, measureclient.EndoL} {
		l[i] = []float64{400, 300}
	}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "eye centres coincide")

	// Finite landmarks whose eye-centre distance overflows to infinity.
	l = syntheticFace(0)
	l[measureclient.ExoR], l[measureclient.EndoR] = []float64{-1e308, 300}, []float64{-1e308, 300}
	l[measureclient.ExoL], l[measureclient.EndoL] = []float64{1e308, 300}, []float64{1e308, 300}
	_, err = guidance.NewFrame(l)
	assert.ErrorContains(t, err, "eye-centre distance is not finite")
}
