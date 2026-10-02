package conversationflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound       = errors.New("conversation flow not found")
	ErrSurveyNotFound = errors.New("survey not found")
	ErrUnknownBrand   = errors.New("unknown brand")
)

// SurveyLookup returns the question types of the active survey, or an error
// wrapping ErrSurveyNotFound.
type SurveyLookup func(ctx context.Context, brandID, applicationID, code string) (QuestionTypes, error)

// ProfileCheck reports whether the brand and application have an active
// face-architecture profile.
type ProfileCheck func(ctx context.Context, brandID, applicationID string) (bool, error)

// BrandResolver returns the canonical ref_brands id for an id or code (or an
// error wrapping ErrUnknownBrand), so a flow saved under either is found by either.
type BrandResolver func(ctx context.Context, ref string) (string, error)

// ValidationError lists every reason a flow was refused.
type ValidationError struct{ Errors []string }

func (e *ValidationError) Error() string {
	return "invalid conversation flow: " + strings.Join(e.Errors, "; ")
}

const warnNoFaceProfile = "no active face-architecture profile for this brand and application: " +
	"the face_architecture step reports unavailable until one is activated"

type Service struct {
	store       Store
	surveys     SurveyLookup
	faceProfile ProfileCheck
	brands      BrandResolver
}

func NewService(store Store, surveys SurveyLookup, faceProfile ProfileCheck, brands BrandResolver) *Service {
	return &Service{store: store, surveys: surveys, faceProfile: faceProfile, brands: brands}
}

func (s *Service) List(ctx context.Context, brandRef, applicationID string) ([]Flow, error) {
	brandID, err := s.brands(ctx, brandRef)
	if err != nil {
		return nil, err
	}
	return s.store.List(ctx, brandID, applicationID)
}

func (s *Service) Active(ctx context.Context, brandRef, applicationID, surveyCode string) (*Flow, error) {
	brandID, err := s.brands(ctx, brandRef)
	if err != nil {
		return nil, err
	}
	f, err := s.store.Active(ctx, brandID, applicationID, surveyCode)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrNotFound
	}
	return f, nil
}

// Create saves f as a new flow. It returns warnings that did not block the save.
func (s *Service) Create(ctx context.Context, f *Flow) ([]string, error) {
	f.ID = ""
	return s.save(ctx, f)
}

// Update replaces flow id with f.
func (s *Service) Update(ctx context.Context, id string, f *Flow) ([]string, error) {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, ErrNotFound
	}
	f.ID, f.CreatedAt = cur.ID, cur.CreatedAt
	return s.save(ctx, f)
}

func (s *Service) save(ctx context.Context, f *Flow) ([]string, error) {
	Normalize(f)
	var errs []string
	questions := QuestionTypes{}
	if f.BrandID != "" && f.ApplicationID != "" && f.SurveyCode != "" {
		brandID, err := s.brands(ctx, f.BrandID)
		if err != nil {
			return nil, err
		}
		f.BrandID = brandID
		q, err := s.surveys(ctx, f.BrandID, f.ApplicationID, f.SurveyCode)
		switch {
		case errors.Is(err, ErrSurveyNotFound):
			errs = append(errs, fmt.Sprintf("survey %q has no active version for this brand and application", f.SurveyCode))
		case err != nil:
			return nil, err
		default:
			questions = q
		}
	}
	errs = append(errs, Validate(f, questions)...)
	if f.Status == StatusActive && f.BrandID != "" {
		cur, err := s.store.Active(ctx, f.BrandID, f.ApplicationID, f.SurveyCode)
		if err != nil {
			return nil, err
		}
		if cur != nil && cur.ID != f.ID {
			errs = append(errs, fmt.Sprintf("flow %s is already active for this survey: archive it first", cur.ID))
		}
	}
	if len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	warnings := []string{}
	if hasEngine(f, EngineFaceArchitecture) {
		ok, err := s.faceProfile(ctx, f.BrandID, f.ApplicationID)
		if err != nil {
			return nil, err
		}
		if !ok {
			warnings = append(warnings, warnNoFaceProfile)
		}
	}
	var err error
	if f.ID == "" {
		err = s.store.Create(ctx, f)
	} else {
		err = s.store.Update(ctx, f)
	}
	return warnings, err
}

func hasEngine(f *Flow, engine string) bool {
	for _, s := range f.Steps {
		if s.Engine == engine {
			return true
		}
	}
	return false
}
