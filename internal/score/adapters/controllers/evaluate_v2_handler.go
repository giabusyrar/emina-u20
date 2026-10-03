package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/service"
)

// EvaluateV2Handler serves the unified evaluate endpoint — :code is a
// ruleset code (not a survey code). Always multipart/form-data since a photo
// may or may not be attached; a data-only (form-only or vision-signal-less)
// call is just the same form with the "photo" field left empty.
type EvaluateV2Handler struct {
	svc *service.ScoreService
}

func NewEvaluateV2Handler(svc *service.ScoreService) *EvaluateV2Handler {
	return &EvaluateV2Handler{svc: svc}
}

// Evaluate handles POST /api/scoring/evaluate/:code
func (h *EvaluateV2Handler) Evaluate(c echo.Context) error {
	code := c.Param("code")

	in := service.EvaluateV2Input{
		RulesetCode:           code,
		BrandID:               c.FormValue("brand_id"),
		ApplicationID:         c.FormValue("application_id"),
		CustomerID:            c.FormValue("customer_id"),
		FullName:              c.FormValue("full_name"),
		PhoneNumber:           c.FormValue("phone_number"),
		Email:                 c.FormValue("email"),
		DateOfBirth:           c.FormValue("date_of_birth"),
		ConsentDataProcessing: c.FormValue("consent_data_processing") == "true",
		ConsentMarketing:      c.FormValue("consent_marketing") == "true",
	}

	if raw := c.FormValue("data"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &in.Data); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid data field: " + err.Error()})
		}
	}

	if fileHeader, err := c.FormFile("photo"); err == nil {
		file, err := fileHeader.Open()
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "could not read photo: " + err.Error()})
		}
		defer file.Close()
		in.Photo = file
		in.PhotoFilename = fileHeader.Filename
	}

	out, err := h.svc.EvaluateV2(c.Request().Context(), in)
	if err != nil {
		if errors.Is(err, domain.ErrVisionUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
				"success":    false,
				"error_code": "VISION_UNAVAILABLE",
				"error":      err.Error(),
			})
		}
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		var verrs *domain.ValidationErrors
		if errors.As(err, &verrs) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"errors": verrs.Errors})
		}
		if requestStatus(err) == http.StatusBadRequest {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"errors": []string{err.Error()}})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, out)
}
