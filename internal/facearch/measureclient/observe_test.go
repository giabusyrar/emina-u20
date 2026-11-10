package measureclient_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	mc "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

func TestObserve_States(t *testing.T) {
	ix := facearchtest.FixtureFace().Index()

	o := ix.Observe("intercanthal_ratio")
	assert.Equal(t, mc.StateUsable, o.State)
	assert.True(t, o.Usable())
	assert.InDelta(t, 1.1374, o.Value, 1e-9)
	require.NotNil(t, o.Band)
	assert.InDelta(t, 0.9301, o.Band[0], 1e-9)
	assert.InDelta(t, 1.4653, o.Band[1], 1e-9)

	assert.Equal(t, mc.StateOccluded, ix.Observe("nose_length_ratio").State)
	assert.Equal(t, mc.StateNotSupported, ix.Observe("eyelid_type").State)
	assert.Equal(t, mc.StateNotScalar, ix.Observe("fifths").State)
	assert.Equal(t, mc.StateMissing, ix.Observe("no_such_key").State)
}

// The worker's "degenerate geometry": value null, visibility unknown.
func TestObserve_NullVisibleValueIsNoValue(t *testing.T) {
	reason := "degenerate geometry"
	r := &mc.Response{Measurements: []mc.Measurement{{Key: "k", Visibility: mc.VisibilityUnknown, Reason: &reason}}}
	o := r.Index().Observe("k")
	assert.Equal(t, mc.StateNoValue, o.State)
	assert.False(t, o.Usable())
}

// A band is null when bands are disabled or too few perturbed samples were
// finite. The value is still usable; there is just no band.
func TestObserve_NullBandIsUsableWithoutBand(t *testing.T) {
	v := 0.5
	r := &mc.Response{Measurements: []mc.Measurement{{Key: "k", Value: mc.Value{Scalar: &v}, Visibility: mc.VisibilityUnknown}}}
	o := r.Index().Observe("k")
	assert.True(t, o.Usable())
	assert.Nil(t, o.Band)
}

func TestCatalogueShape(t *testing.T) {
	cat := facearchtest.Catalogue()
	s, err := cat.Shape("fifths")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeList, s)
	s, err = cat.Shape("jaw_contour_angles")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeList, s)
	s, err = cat.Shape("width_length_ratio")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeScalar, s)

	_, err = cat.Shape("no_such_key")
	assert.Error(t, err)

	other := &mc.Catalogue{CatalogueVersion: "fa-measure/2", Measurements: []mc.CatalogueEntry{{Key: "a"}, {Key: "b", Shape: "matrix"}}}
	_, err = other.Shape("a")
	assert.ErrorIs(t, err, mc.ErrShapeUnknown, "no shape is refused, not guessed")
	_, err = other.Shape("b")
	assert.ErrorIs(t, err, mc.ErrShapeUnknown, "an unrecognised shape is refused")
}

// Every non-null recorded value is a list exactly when the catalogue says so.
func TestCatalogueShapeMatchesRecordedValues(t *testing.T) {
	cat := facearchtest.Catalogue()
	for _, m := range facearchtest.FixtureFace().Measurements {
		if m.Value.IsNull() {
			continue
		}
		s, err := cat.Shape(m.Key)
		require.NoError(t, err, m.Key)
		assert.Equal(t, m.Value.IsList(), s == mc.ShapeList, m.Key)
	}
}

// Plan 1's bands are value-centred (value + P5-P50, value + P95-P50), so
// every recorded band contains its value, element by element for lists.
func TestRecordedBandsContainTheirValue(t *testing.T) {
	for _, m := range facearchtest.FixtureFace().Measurements {
		if m.Band == nil {
			continue
		}
		lo, hi := m.Band[0], m.Band[1]
		if m.Value.IsList() {
			require.Len(t, lo.List, len(m.Value.List), m.Key)
			require.Len(t, hi.List, len(m.Value.List), m.Key)
			for i, v := range m.Value.List {
				assert.LessOrEqual(t, lo.List[i], v, m.Key)
				assert.LessOrEqual(t, v, hi.List[i], m.Key)
			}
			continue
		}
		require.NotNil(t, m.Value.Scalar, m.Key)
		assert.LessOrEqual(t, *lo.Scalar, *m.Value.Scalar, m.Key)
		assert.LessOrEqual(t, *m.Value.Scalar, *hi.Scalar, m.Key)
	}
}
