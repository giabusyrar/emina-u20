package controllers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/pkg/form/domain"
)

// respondErr maps a service-layer error to an HTTP status + JSON
// body. Validation failures collect every rule violation (Section 11)
// instead of failing fast on the first one.
func respondErr(c echo.Context, err error) error {
	var verrs *domain.ValidationErrors
	if errors.As(err, &verrs) {
		return c.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"success": false,
			"errors":  verrs.Errors,
		})
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]interface{}{"success": false, "error": "not found"})
	case errors.Is(err, domain.ErrConflict):
		return c.JSON(http.StatusConflict, map[string]interface{}{"success": false, "error": err.Error()})
	case errors.Is(err, domain.ErrInvalidCode):
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
	case errors.Is(err, domain.ErrUpstream):
		return c.JSON(http.StatusBadGateway, map[string]interface{}{"success": false, "error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
}
