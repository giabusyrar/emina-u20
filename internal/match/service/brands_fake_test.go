package service_test

import (
	"context"
	"fmt"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
)

// fakeBrands resolves the brands the in-memory test catalogue uses. Their
// products carry the code as brand id, so id and code are the same here.
type fakeBrands map[string]bool

func (f fakeBrands) Resolve(_ context.Context, ref string) (brand.Brand, error) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if ref == "" {
		return brand.Brand{}, brand.ErrRequired
	}
	if !f[ref] {
		return brand.Brand{}, fmt.Errorf("%w: %q", brand.ErrUnknown, ref)
	}
	return brand.Brand{ID: ref, Code: ref}, nil
}

var testBrands = fakeBrands{"wardah": true, "makeover": true, "omg": true}
