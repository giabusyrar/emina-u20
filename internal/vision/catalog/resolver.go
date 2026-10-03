package catalog

import (
	"context"
	"fmt"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

// Plan is what one analyze request will compute.
type Plan struct {
	// Conditions: requested conditions plus every condition under a requested
	// dimension (deduplicated, request order).
	Conditions []Condition
	// Dimensions that receive a rolled-up score: only requested dimensions,
	// or every dimension when the request names nothing.
	Dimensions []string
	// Capabilities: union of the Conditions' capability codes.
	Capabilities []string
}

// ValidationError lists request codes that are not in Reference Data.
type ValidationError struct {
	Items []domain.ValidationErrorItem
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d unrecognized dimension or skin condition code(s)", len(e.Items))
}

// Resolver validates requested codes against Reference Data and builds a Plan.
type Resolver struct {
	catalog Catalog
}

func NewResolver(catalog Catalog) *Resolver {
	return &Resolver{catalog: catalog}
}

func (r *Resolver) Resolve(ctx context.Context, dimensions, skinConditions []string) (*Plan, error) {
	snapshot, err := r.catalog.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return resolvePlan(snapshot, dimensions, skinConditions)
}

func resolvePlan(snapshot *Snapshot, dimensions, skinConditions []string) (*Plan, error) {
	dimensionsByKey := make(map[string]Dimension, len(snapshot.Dimensions))
	for _, d := range snapshot.Dimensions {
		dimensionsByKey[normalizeCode(d.Code)] = d
	}
	conditionsByKey := make(map[string]Condition, len(snapshot.Conditions))
	for _, c := range snapshot.Conditions {
		conditionsByKey[normalizeCode(c.Code)] = c
	}

	var invalid []domain.ValidationErrorItem

	var requestedDimensions []Dimension
	seenDimensions := make(map[string]bool)
	for _, raw := range dimensions {
		key := normalizeCode(raw)
		if key == "" {
			continue
		}
		d, ok := dimensionsByKey[key]
		if !ok {
			invalid = append(invalid, domain.ValidationErrorItem{
				Field:  "dimensions",
				Value:  raw,
				Reason: fmt.Sprintf("unrecognized dimension. Supported: [%s]", strings.Join(dimensionCodes(snapshot), ", ")),
			})
			continue
		}
		if !seenDimensions[d.Code] {
			seenDimensions[d.Code] = true
			requestedDimensions = append(requestedDimensions, d)
		}
	}

	var requestedConditions []Condition
	for _, raw := range skinConditions {
		key := normalizeCode(raw)
		if key == "" {
			continue
		}
		c, ok := conditionsByKey[key]
		if !ok {
			invalid = append(invalid, domain.ValidationErrorItem{
				Field:  "skinConditions",
				Value:  raw,
				Reason: fmt.Sprintf("unrecognized skin condition. Supported: [%s]", strings.Join(conditionCodes(snapshot), ", ")),
			})
			continue
		}
		requestedConditions = append(requestedConditions, c)
	}

	if len(invalid) > 0 {
		return nil, &ValidationError{Items: invalid}
	}

	var candidates []Condition
	if len(requestedDimensions) == 0 && len(requestedConditions) == 0 {
		requestedDimensions = snapshot.Dimensions
		candidates = snapshot.Conditions
	} else {
		for _, d := range requestedDimensions {
			candidates = append(candidates, d.Conditions...)
		}
		candidates = append(candidates, requestedConditions...)
	}

	plan := &Plan{}
	seenConditions := make(map[string]bool)
	seenCapabilities := make(map[string]bool)
	for _, c := range candidates {
		if seenConditions[c.Code] {
			continue
		}
		seenConditions[c.Code] = true
		plan.Conditions = append(plan.Conditions, c)
		for _, capability := range c.Capabilities {
			if !seenCapabilities[capability] {
				seenCapabilities[capability] = true
				plan.Capabilities = append(plan.Capabilities, capability)
			}
		}
	}
	for _, d := range requestedDimensions {
		plan.Dimensions = append(plan.Dimensions, d.Code)
	}

	return plan, nil
}

func normalizeCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func dimensionCodes(snapshot *Snapshot) []string {
	codes := make([]string, len(snapshot.Dimensions))
	for i, d := range snapshot.Dimensions {
		codes[i] = d.Code
	}
	return codes
}

func conditionCodes(snapshot *Snapshot) []string {
	codes := make([]string, len(snapshot.Conditions))
	for i, c := range snapshot.Conditions {
		codes[i] = c.Code
	}
	return codes
}
