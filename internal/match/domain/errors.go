package domain

import (
	"errors"
	"strings"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record already exists")
)

type ValidationErrors struct {
	Errors []string
}

func (v *ValidationErrors) Add(err string) {
	v.Errors = append(v.Errors, err)
}

func (v *ValidationErrors) HasErrors() bool {
	return len(v.Errors) > 0
}

func (v *ValidationErrors) Error() string {
	return strings.Join(v.Errors, "; ")
}
