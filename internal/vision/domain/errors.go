package domain

import (
	"errors"
	"fmt"
)

// The pipeline's failure kinds. The infrastructure packages re-export these
// under their own names (catalog.ErrCatalogUnavailable and so on), so
// callers match them with errors.Is without importing that infrastructure.
var (
	// ErrCatalogUnavailable means Reference Data could not be loaded and no
	// earlier snapshot exists to fall back on.
	ErrCatalogUnavailable = errors.New("reference data unavailable")

	// ErrSegmentationUnavailable: the skin worker could not segment the image
	// (unreachable, no model loaded, or an unusable reply). There is no
	// substitute: callers report it instead of using template zones.
	ErrSegmentationUnavailable = errors.New("skin segmentation unavailable")

	// ErrNoFace: the skin worker measured the image and found no face.
	ErrNoFace = errors.New("no face detected")

	// ErrWorkerUnavailable wraps every failure to get scores from the vision
	// AI worker.
	ErrWorkerUnavailable = errors.New("vision AI worker unavailable")
)

// ValidationError lists request codes that are not in Reference Data.
type ValidationError struct {
	Items []ValidationErrorItem
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d unrecognized dimension or skin condition code(s)", len(e.Items))
}
