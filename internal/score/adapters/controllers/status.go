package controllers

import (
	"errors"
	"net/http"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
)

// requestStatus is 400 for a request that cannot be served as sent (failed
// validation, a brand ref_brands does not know) and 500 for anything else.
func requestStatus(err error) int {
	var verrs *domain.ValidationErrors
	if errors.As(err, &verrs) || errors.Is(err, brand.ErrRequired) || errors.Is(err, brand.ErrUnknown) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
