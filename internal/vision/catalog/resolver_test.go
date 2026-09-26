package catalog

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticCatalog struct {
	snapshot *Snapshot
	err      error
}

func (s staticCatalog) Snapshot(ctx context.Context) (*Snapshot, error) {
	return s.snapshot, s.err
}

func testSnapshot() *Snapshot {
	papule := Condition{Code: "concern_acne_papule", DimensionCode: "acne", Capabilities: []string{"acne_lesion_classifier"}}
	blackhead := Condition{Code: "concern_blackhead", DimensionCode: "acne", Capabilities: []string{"comedone_pore_detector"}}
	melasma := Condition{Code: "concern_melasma", DimensionCode: "pigmentation", Capabilities: []string{"hyperpigmentation_net"}}
	pih := Condition{Code: "concern_pih", DimensionCode: "pigmentation", Capabilities: []string{"hyperpigmentation_net"}}
	darkCircles := Condition{Code: "Concern_Dark_Circles", DimensionCode: "Eye_Area", Capabilities: []string{"periorbital_darkness_detector"}}

	return &Snapshot{
		Dimensions: []Dimension{
			{Code: "acne", Conditions: []Condition{papule, blackhead}},
			{Code: "pigmentation", Conditions: []Condition{melasma, pih}},
			{Code: "Eye_Area", Conditions: []Condition{darkCircles}},
		},
		Conditions: []Condition{papule, blackhead, melasma, pih, darkCircles},
	}
}

func resolveWith(dimensions, skinConditions []string) (*Plan, error) {
	return NewResolver(staticCatalog{snapshot: testSnapshot()}).Resolve(context.Background(), dimensions, skinConditions)
}

func conditionCodesOf(plan *Plan) []string {
	var codes []string
	for _, c := range plan.Conditions {
		codes = append(codes, c.Code)
	}
	return codes
}

func TestResolve_EmptyRequestPlansEverything(t *testing.T) {
	plan, err := resolveWith(nil, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"acne", "pigmentation", "Eye_Area"}, plan.Dimensions)
	assert.Equal(t, []string{"concern_acne_papule", "concern_blackhead", "concern_melasma", "concern_pih", "Concern_Dark_Circles"}, conditionCodesOf(plan))
	assert.Equal(t, []string{"acne_lesion_classifier", "comedone_pore_detector", "hyperpigmentation_net", "periorbital_darkness_detector"}, plan.Capabilities)
}

func TestResolve_BlankCodesCountAsEmptyRequest(t *testing.T) {
	plan, err := resolveWith([]string{" "}, []string{""})
	require.NoError(t, err)

	assert.Len(t, plan.Dimensions, 3)
	assert.Len(t, plan.Conditions, 5)
}

func TestResolve_DimensionExpandsToItsConditions(t *testing.T) {
	plan, err := resolveWith([]string{"pigmentation"}, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"pigmentation"}, plan.Dimensions)
	assert.Equal(t, []string{"concern_melasma", "concern_pih"}, conditionCodesOf(plan))
	assert.Equal(t, []string{"hyperpigmentation_net"}, plan.Capabilities)
}

func TestResolve_ConditionsOnlyProduceNoDimensionScores(t *testing.T) {
	plan, err := resolveWith(nil, []string{"concern_blackhead"})
	require.NoError(t, err)

	assert.Empty(t, plan.Dimensions)
	assert.Equal(t, []string{"concern_blackhead"}, conditionCodesOf(plan))
	assert.Equal(t, []string{"comedone_pore_detector"}, plan.Capabilities)
}

func TestResolve_OverlappingRequestsAreDeduplicated(t *testing.T) {
	plan, err := resolveWith([]string{"acne"}, []string{"concern_acne_papule", "CONCERN_ACNE_PAPULE"})
	require.NoError(t, err)

	assert.Equal(t, []string{"concern_acne_papule", "concern_blackhead"}, conditionCodesOf(plan))
}

func TestResolve_CaseInsensitiveMatchUsesReferenceSpelling(t *testing.T) {
	plan, err := resolveWith([]string{"eye_area"}, []string{"concern_dark_circles"})
	require.NoError(t, err)

	assert.Equal(t, []string{"Eye_Area"}, plan.Dimensions)
	assert.Equal(t, []string{"Concern_Dark_Circles"}, conditionCodesOf(plan))
}

func TestResolve_UnknownAndSubstringCodesAreRejected(t *testing.T) {
	_, err := resolveWith([]string{"pigment"}, []string{"acne"})

	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr))
	require.Len(t, validationErr.Items, 2)
	assert.Equal(t, "dimensions", validationErr.Items[0].Field)
	assert.Equal(t, "pigment", validationErr.Items[0].Value)
	assert.Contains(t, validationErr.Items[0].Reason, "acne, pigmentation, Eye_Area")
	assert.Equal(t, "skinConditions", validationErr.Items[1].Field)
	assert.Equal(t, "acne", validationErr.Items[1].Value)
}

func TestResolve_CatalogErrorIsReturned(t *testing.T) {
	resolver := NewResolver(staticCatalog{err: fmt.Errorf("%w: connection refused", ErrCatalogUnavailable)})

	_, err := resolver.Resolve(context.Background(), []string{"acne"}, nil)

	assert.True(t, errors.Is(err, ErrCatalogUnavailable))
}
