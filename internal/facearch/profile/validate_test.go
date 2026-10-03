package profile_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
)

func problems(t *testing.T, err error) string {
	t.Helper()
	var ve *profile.ValidationError
	require.True(t, errors.As(err, &ve), "want *ValidationError, got %v", err)
	return ve.Error()
}

func TestValidate_ValidDocument(t *testing.T) {
	assert.NoError(t, profile.Validate(parseValid(t), facearchtest.Catalogue()))
	assert.NoError(t, profile.Validate(parseValid(t), nil))
}

func TestValidate_Rules(t *testing.T) {
	cases := map[string]struct {
		mutate func(d *profile.Document)
		want   string
	}{
		"schema version": {func(d *profile.Document) { d.SchemaVersion = 2 }, "schemaVersion: got 2"},
		"catalogue version": {func(d *profile.Document) { d.CatalogueVersion = "fa-measure/0" },
			"document targets fa-measure/0, ai-worker catalogue is fa-measure/1"},
		"unknown key": {func(d *profile.Document) { d.MeasurementsShown = append(d.MeasurementsShown, "golden_ratio") },
			`"golden_ratio" is not in catalogue`},
		"unsupported key": {func(d *profile.Document) { d.MeasurementsShown = append(d.MeasurementsShown, "eyelid_type") },
			`"eyelid_type" is not supported`},
		"list-valued prototype": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["fifths"] = profile.PrototypeTerm{Centre: 0.2, Spread: 0.1, Weight: 1}
		}, `"fifths" is list-valued`},
		"list-valued rule": {func(d *profile.Document) { d.Traits[0].Rules[0].When[0].Measurement = "jaw_contour_angles" },
			`"jaw_contour_angles" is list-valued`},
		"zero spread": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["width_length_ratio"] = profile.PrototypeTerm{Centre: 0.6, Spread: 0, Weight: 1}
		}, "spread: must be positive"},
		"zero weight": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["width_length_ratio"] = profile.PrototypeTerm{Centre: 0.6, Spread: 0.1, Weight: 0}
		}, "weight: must be positive"},
		"blend margin 0.5":        {func(d *profile.Document) { d.Classifiers[0].BlendMargin = 0.5 }, "blendMargin: must be in (0.5, 1]"},
		"blend margin above 1":    {func(d *profile.Document) { d.Classifiers[0].BlendMargin = 1.01 }, "blendMargin: must be in (0.5, 1]"},
		"min observed weight":     {func(d *profile.Document) { d.Classifiers[0].MinObservedWeight = -0.1 }, "minObservedWeight: must be in [0, 1]"},
		"one class":               {func(d *profile.Document) { d.Classifiers[0].Classes = d.Classifiers[0].Classes[:1] }, "needs at least 2 classes"},
		"duplicate class":         {func(d *profile.Document) { d.Classifiers[0].Classes[1].Label = "Long" }, `duplicate class "Long"`},
		"bad op":                  {func(d *profile.Document) { d.Traits[0].Rules[0].When[0].Op = "==" }, "must be one of < <= > >="},
		"empty default":           {func(d *profile.Document) { d.Traits[0].Default = "" }, "default: is empty"},
		"bad calibration status":  {func(d *profile.Document) { d.Calibration.Status = "guessed" }, "calibration.status"},
		"calibrated needs run id": {func(d *profile.Document) { d.Calibration.Status = "calibrated" }, "benchmarkRunId: required"},
		"uncited class": {func(d *profile.Document) { d.Calibration.Sources = d.Calibration.Sources[:2] },
			"no source cited for classifiers.faceShape.classes.Long"},
		"uncited trait": {func(d *profile.Document) { d.Calibration.Sources = d.Calibration.Sources[:4] },
			"no source cited for traits.noseWidth"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := parseValid(t)
			tc.mutate(d)
			msg := problems(t, profile.Validate(d, facearchtest.Catalogue()))
			assert.Contains(t, msg, tc.want)
		})
	}
}

func TestValidate_ParentSourceDoesNotCoverChildren(t *testing.T) {
	d := parseValid(t)
	d.Calibration.Sources = []profile.Source{{Cite: "vocabulary only", For: "traits"}, {Cite: "x", For: "classifiers"}}
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "traits.noseWidth")
	assert.Contains(t, msg, "classifiers.faceShape.blendMargin")
}

func TestValidate_ReportsEveryProblem(t *testing.T) {
	d := parseValid(t)
	d.SchemaVersion = 0
	d.Classifiers[0].BlendMargin = 2
	d.Traits[0].Default = ""
	var ve *profile.ValidationError
	require.True(t, errors.As(profile.Validate(d, nil), &ve))
	paths := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		paths[i] = p.Path
	}
	assert.Equal(t, "schemaVersion,classifiers[0].blendMargin,traits[0].default", strings.Join(paths, ","))
}

func TestLoad(t *testing.T) {
	d, err := profile.Load(&profile.FaceArchitectureProfile{Document: validDoc})
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", d.CatalogueVersion)

	_, err = profile.Load(&profile.FaceArchitectureProfile{Document: `{"schemaVersion": 7}`})
	assert.Error(t, err)
}
