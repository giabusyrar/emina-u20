package domain

import (
	"errors"
	"strings"
)

var (
	// ErrWorkerUnavailable: the colour worker is unreachable, timed out, or has
	// not loaded its model. Maps to 503.
	ErrWorkerUnavailable = errors.New("colour worker unavailable")
	// ErrNoFace: the worker found no usable face. Maps to 422.
	ErrNoFace = errors.New("no face detected")
	// ErrMeasurementFailed: a measurement stage failed on this photo. Maps to 422.
	ErrMeasurementFailed = errors.New("measurement failed")
	// ErrRetakeRequired: retake.enforce is on and the photo failed a retake QC
	// check. Maps to 422; the error is a *RetakeRequiredError carrying the reasons.
	ErrRetakeRequired = errors.New("retake required")
	// ErrInvalidInput: bad image, bad field. Maps to 400.
	ErrInvalidInput = errors.New("invalid input")
	// ErrShadeNotFound: unknown shade id, or a shade without colorimetry. Maps to 404.
	ErrShadeNotFound = errors.New("shade not found")
)

// RetakeRequiredError is ErrRetakeRequired plus the QC codes that caused it.
type RetakeRequiredError struct{ Reasons []string }

func (e *RetakeRequiredError) Error() string {
	return ErrRetakeRequired.Error() + ": " + strings.Join(e.Reasons, ", ")
}

func (e *RetakeRequiredError) Unwrap() error { return ErrRetakeRequired }
