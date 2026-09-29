package domain

import "errors"

var (
	// ErrWorkerUnavailable: the colour worker is unreachable, timed out, or has
	// not loaded its model. Maps to 503.
	ErrWorkerUnavailable = errors.New("colour worker unavailable")
	// ErrNoFace: the worker found no usable face. Maps to 422.
	ErrNoFace = errors.New("no face detected")
	// ErrMeasurementFailed: a measurement stage failed on this photo. Maps to 422.
	ErrMeasurementFailed = errors.New("measurement failed")
	// ErrInvalidInput: bad image, bad field. Maps to 400.
	ErrInvalidInput = errors.New("invalid input")
	// ErrShadeNotFound: unknown shade id, or a shade without colorimetry. Maps to 404.
	ErrShadeNotFound = errors.New("shade not found")
)
