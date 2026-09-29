package profile

import (
	"context"
	"errors"
)

// ErrNoActiveProfile means no active profile matches brand+app, brand+*,
// or *+*. There are no built-in defaults; the handler answers 409.
var ErrNoActiveProfile = errors.New("no_active_face_architecture_profile")

// UpsertInput is a validated document and the scope to store it under.
type UpsertInput struct {
	BrandID       string
	ApplicationID string
	Code          string
	Title         string
	Document      []byte
}

// Store finds and writes profiles.
type Store interface {
	// Active returns the highest active version for the first scope in
	// Scopes(brandID, appID) that has one, or ErrNoActiveProfile.
	Active(ctx context.Context, brandID, appID string) (*FaceArchitectureProfile, error)
	// Upsert inserts the document as the next version of (brand, app,
	// code), makes it ACTIVE, and marks every other active profile in the
	// same brand+app scope INACTIVE, so each scope has one active profile.
	Upsert(ctx context.Context, in UpsertInput) (*FaceArchitectureProfile, error)
}

// Scopes is the lookup order: brand+app, brand+*, *+*. Empty values mean
// Wildcard; a scope that repeats an earlier one is left out.
func Scopes(brandID, appID string) [][2]string {
	if brandID == "" {
		brandID = Wildcard
	}
	if appID == "" {
		appID = Wildcard
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, s := range [][2]string{{brandID, appID}, {brandID, Wildcard}, {Wildcard, Wildcard}} {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func normaliseInput(in UpsertInput) (UpsertInput, error) {
	if in.BrandID == "" || in.ApplicationID == "" || in.Code == "" {
		return in, errors.New("brand, application and code are required (use * for any brand or application)")
	}
	if in.Title == "" {
		in.Title = in.Code
	}
	return in, nil
}
