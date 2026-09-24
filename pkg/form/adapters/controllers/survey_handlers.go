package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/pkg/form/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/form/service"
)

type SurveyHandler struct {
	surveyService *service.SurveyService
}

func NewSurveyHandler(surveyService *service.SurveyService) *SurveyHandler {
	return &SurveyHandler{surveyService: surveyService}
}

func (h *SurveyHandler) Get(c echo.Context) error {
	code := c.Param("code")
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")

	if brandID == "" || appID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "brand_id and application_id query parameters are required"})
	}

	model, err := h.surveyService.GetSurvey(c.Request().Context(), brandID, appID, code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "survey not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, model)
}

func (h *SurveyHandler) List(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")

	surveys, err := h.surveyService.ListSurveys(c.Request().Context(), brandID, appID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, surveys)
}

type SaveSurveyRequest struct {
	BrandID       string          `json:"brand_id"`
	ApplicationID string          `json:"application_id"`
	Code          string          `json:"code"`
	Title         string          `json:"title"`
	Status        string          `json:"status"`
	Schema        json.RawMessage `json:"schema"`
}

func (h *SurveyHandler) Save(c echo.Context) error {
	var req SaveSurveyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
	}

	if codeParam := c.Param("code"); codeParam != "" {
		req.Code = codeParam
	}

	if req.BrandID == "" || req.ApplicationID == "" || req.Code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "brand_id, application_id, and code are required"})
	}

	if err := h.surveyService.SaveSurvey(c.Request().Context(), req.BrandID, req.ApplicationID, req.Code, req.Title, req.Status, string(req.Schema)); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"code":    req.Code,
		"status":  req.Status,
	})
}

func (h *SurveyHandler) Evaluate(c echo.Context) error {
	code := c.Param("code")
	var req domain.SurveyEvaluateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	out, err := h.surveyService.Evaluate(c.Request().Context(), code, req)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "survey not found"})
		}
		var verrs *domain.ValidationErrors
		if errors.As(err, &verrs) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"errors": verrs.Errors})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, out)
}

// EvaluateWithPhotos handles POST /survey/:code/evaluate-with-photos — the
// one call a brand's Submit button makes: personal data + Q1-Q6 answers +
// a photo, multipart/form-data (the photo can't travel as JSON). Internally
// this calls Paradev itself and folds the result into the same evaluate
// pipeline Evaluate above uses.
func (h *SurveyHandler) EvaluateWithPhotos(c echo.Context) error {
	code := c.Param("code")

	req := domain.EvaluateWithPhotosRequest{
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
		if err := json.Unmarshal([]byte(raw), &req.Data); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid data field: " + err.Error()})
		}
	}
	if raw := c.FormValue("customer_conditions"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &req.CustomerConditions); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid customer_conditions field: " + err.Error()})
		}
	}

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "a photo (field \"photo\") is required"})
	}
	file, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "could not read photo: " + err.Error()})
	}
	defer file.Close()

	out, err := h.surveyService.EvaluateWithPhotos(c.Request().Context(), code, req, file, fileHeader.Filename)
	if err != nil {
		if errors.Is(err, domain.ErrVisionUnavailable) {
			return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
				"success":    false,
				"error_code": "VISION_UNAVAILABLE",
				"error":      err.Error(),
			})
		}
		if errors.Is(err, domain.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "survey not found"})
		}
		var verrs *domain.ValidationErrors
		if errors.As(err, &verrs) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"errors": verrs.Errors})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, out)
}
