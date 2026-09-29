package measureclient_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestDecodesScalarListAndNullValues(t *testing.T) {
	resp := facearchtest.FixtureFace()
	byKey := map[string]mc.Measurement{}
	for _, m := range resp.Measurements {
		byKey[m.Key] = m
	}
	eye := byKey["eye_width"]
	require.NotNil(t, eye.Value.Scalar)
	assert.InDelta(t, 0.4679, *eye.Value.Scalar, 1e-9)
	require.NotNil(t, eye.Band)
	assert.InDelta(t, 0.3996, *eye.Band[0].Scalar, 1e-9)
	assert.Nil(t, eye.Reason)

	fifths := byKey["fifths"]
	assert.True(t, fifths.Value.IsList())
	assert.Len(t, fifths.Value.List, 5)
	assert.Len(t, fifths.Band[0].List, 5)

	jaw := byKey["jaw_width_proxy"]
	assert.True(t, jaw.Value.IsNull())
	assert.Nil(t, jaw.Band)
	require.NotNil(t, jaw.Reason)
	assert.Equal(t, "requires region jaw (out of frame)", *jaw.Reason)

	assert.True(t, byKey["alar_width_to_intercanthal_ratio"].Proxy, "alar anchors are nostril-base proxies")
	assert.Len(t, resp.Landmarks, 478)
	assert.Len(t, resp.Landmarks[0], 2)
	_, hasJaw := resp.Quality.RegionDeltaE["jaw"]
	assert.False(t, hasJaw, "regionDeltaE omits out-of-frame regions")
}

func TestMeasurement_RoundTripsJSON(t *testing.T) {
	var outer struct {
		Measurements []json.RawMessage `json:"measurements"`
	}
	require.NoError(t, json.Unmarshal(facearchtest.FixtureFaceJSON(), &outer))
	for _, raw := range outer.Measurements {
		var m mc.Measurement
		require.NoError(t, json.Unmarshal(raw, &m))
		again, err := json.Marshal(m)
		require.NoError(t, err)
		assert.JSONEq(t, string(raw), string(again))
	}
}

// Every catalogue field the worker sends, and only those, survives a
// decode/encode: CatalogueEntry mirrors CatalogueEntryOut exactly.
func TestCatalogueEntry_RoundTripsJSON(t *testing.T) {
	var outer struct {
		Measurements []json.RawMessage `json:"measurements"`
	}
	require.NoError(t, json.Unmarshal(facearchtest.CatalogueJSON(), &outer))
	require.NotEmpty(t, outer.Measurements)
	for _, raw := range outer.Measurements {
		var e mc.CatalogueEntry
		require.NoError(t, json.Unmarshal(raw, &e))
		again, err := json.Marshal(e)
		require.NoError(t, err)
		assert.JSONEq(t, string(raw), string(again))
	}
}
