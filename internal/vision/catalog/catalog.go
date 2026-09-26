package catalog

import (
	"context"
	"errors"
)

// Condition is a vision-relevant skin condition from Reference Data: it maps
// to at least one vision capability code.
type Condition struct {
	Code          string
	Name          string
	Description   string
	DimensionCode string
	Capabilities  []string
}

// Dimension is a vision-relevant dimension: it has at least one Condition.
type Dimension struct {
	Code        string
	Name        string
	Description string
	Conditions  []Condition
}

// Snapshot is the vision-relevant subset of Reference Data, in Reference order.
type Snapshot struct {
	Dimensions []Dimension
	Conditions []Condition
}

// ErrCatalogUnavailable means Reference Data could not be loaded and no
// earlier snapshot exists to fall back on.
var ErrCatalogUnavailable = errors.New("reference data unavailable")

// Catalog serves the current Reference Data snapshot.
type Catalog interface {
	Snapshot(ctx context.Context) (*Snapshot, error)
}
