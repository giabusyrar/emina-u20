package measureclient

import (
	"errors"
	"fmt"
)

// Shapes a measurement value can have.
const (
	ShapeScalar = "scalar"
	ShapeList   = "list"
)

// ErrShapeUnknown means the catalogue entry does not say whether the
// measurement is a number or a list.
var ErrShapeUnknown = errors.New("measurement shape unknown")

// Shape returns ShapeScalar or ShapeList for key, as the catalogue entry
// declares it. The unit cannot tell ("fifths" is a "ratio" and
// "jaw_contour_angles" a "deg", like scalar keys), so core-engine keeps no
// table of its own: an entry without a recognised shape is refused
// (ErrShapeUnknown) rather than guessed.
func (c *Catalogue) Shape(key string) (string, error) {
	e, ok := c.Entry(key)
	if !ok {
		return "", fmt.Errorf("measurement %q is not in catalogue %s", key, c.CatalogueVersion)
	}
	switch e.Shape {
	case ShapeScalar, ShapeList:
		return e.Shape, nil
	}
	return "", fmt.Errorf("%w: measurement %q in catalogue %s has shape %q", ErrShapeUnknown, key, c.CatalogueVersion, e.Shape)
}
