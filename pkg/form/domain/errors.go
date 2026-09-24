package domain

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("already exists")
	ErrDeprecated     = errors.New("deprecated")
	ErrInvalidCode    = errors.New("code must be alphanumeric, max 10 characters")
	ErrUpstream       = errors.New("upstream request failed")
	// ErrVisionUnavailable means the vision provider (Paradev) failed or
	// timed out. Surfaced as its own error, not a partial result, because an
	// axis like W/T (Aging) has no meaning without a vision-side value.
	ErrVisionUnavailable = errors.New("vision provider unavailable")
)

// ValidationErrors collects every rule violation found while
// validating a mapping publish or an evaluate submission, so the
// caller gets the full picture in one response instead of failing
// fast on the first issue.
type ValidationErrors struct {
	Errors []string
}

func (e *ValidationErrors) Add(msg string) {
	e.Errors = append(e.Errors, msg)
}

func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return ""
	}
	out := e.Errors[0]
	for _, m := range e.Errors[1:] {
		out += "; " + m
	}
	return out
}
