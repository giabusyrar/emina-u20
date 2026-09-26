package controllers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
	"github.com/experience-gateway/packages/goplatform"
)

// respondErr maps a service-layer error to an HTTP status + JSON
// body. Validation failures collect every rule violation (Section 11)
// instead of failing fast on the first one.
func respondErr(c echo.Context, err error) error {
	var verrs *domain.ValidationErrors
	if errors.As(err, &verrs) {
		return goplatform.JSONValidationError(c, http.StatusUnprocessableEntity, verrs.Errors)
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return goplatform.JSONError(c, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrConflict):
		return goplatform.JSONError(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidCode):
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUpstream):
		return goplatform.JSONError(c, http.StatusBadGateway, err.Error())
	default:
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
}
