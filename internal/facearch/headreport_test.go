package facearch_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
)

func TestHeadReport_FixtureRoundTripsWithoutUnknownFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/head_report.json")
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r facearch.HeadReport
	require.NoError(t, dec.Decode(&r))
	out, err := json.Marshal(r)
	require.NoError(t, err)
	var a, b map[string]any
	require.NoError(t, json.Unmarshal(raw, &a))
	require.NoError(t, json.Unmarshal(out, &b))
	assert.Equal(t, a, b)
}
