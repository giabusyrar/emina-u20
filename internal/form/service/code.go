package service

import (
	"regexp"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

var codePattern = regexp.MustCompile(`^[A-Za-z0-9]{1,10}$`)

// ValidateCode enforces Section 11's rule #1: required, alphanumeric,
// max 10 characters. Shared by questionnaire codes and every
// registry's condition_key/dimension_key/parameter_key.
func ValidateCode(code string) error {
	if !codePattern.MatchString(code) {
		return domain.ErrInvalidCode
	}
	return nil
}
