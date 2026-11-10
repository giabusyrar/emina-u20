// Package brand resolves the brand a request names into the one brand row in
// ref_brands (reference-service owns that table; core-engine only reads it).
//
// Two spellings of a brand are in use: its ref_brands id ("brd-…"), which
// products carry, and its code ("WARDAH"), which surveys, rulesets and match
// blueprints are keyed by in lower case ("wardah"). A request may send
// either; Resolve returns both, so each lookup uses the form its data is
// keyed by. A brand that is not in ref_brands is an error, never a default.
package brand

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Brand is one ref_brands row. Code is lower-cased: the form the
// code-keyed data (surveys, rulesets, blueprints) is stored under.
type Brand struct {
	ID   string
	Code string
}

var (
	// ErrRequired: the request named no brand.
	ErrRequired = errors.New("brand_id is required")
	// ErrUnknown: no ref_brands row has that id or code.
	ErrUnknown = errors.New("unknown brand")
)

// Resolver looks brands up in ref_brands.
type Resolver struct {
	db *gorm.DB
}

func NewResolver(db *gorm.DB) *Resolver { return &Resolver{db: db} }

// Resolve finds the brand whose id equals ref or whose code equals ref
// ignoring case. Errors wrap ErrRequired or ErrUnknown for the caller to
// map to 400; anything else is a database error.
func (r *Resolver) Resolve(ctx context.Context, ref string) (Brand, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Brand{}, ErrRequired
	}
	if r == nil || r.db == nil {
		return Brand{}, fmt.Errorf("brand resolver: no database")
	}
	var rows []struct{ ID, Code string }
	if err := r.db.WithContext(ctx).Raw(
		`SELECT id, code FROM ref_brands WHERE id = ? OR LOWER(code) = LOWER(?) LIMIT 2`, ref, ref,
	).Scan(&rows).Error; err != nil {
		return Brand{}, fmt.Errorf("resolving brand %q: %w", ref, err)
	}
	return pick(ref, rows)
}

// pick turns the matching rows into one brand: none is unknown, two (an id
// that is another brand's code) is refused rather than guessed.
func pick(ref string, rows []struct{ ID, Code string }) (Brand, error) {
	switch len(rows) {
	case 0:
		return Brand{}, fmt.Errorf("%w: %q is neither a ref_brands id nor a brand code", ErrUnknown, ref)
	case 1:
		return Brand{ID: rows[0].ID, Code: strings.ToLower(strings.TrimSpace(rows[0].Code))}, nil
	}
	return Brand{}, fmt.Errorf("%w: %q matches more than one brand", ErrUnknown, ref)
}
