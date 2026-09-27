package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
	"github.com/experience-gateway/apps/core-engine/internal/form/entity"
	"github.com/experience-gateway/apps/core-engine/internal/form/service"
	"github.com/experience-gateway/packages/goplatform"
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
		return goplatform.JSONError(c, http.StatusBadRequest, "brand_id and application_id query parameters are required")
	}

	model, err := h.surveyService.GetSurvey(c.Request().Context(), brandID, appID, code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return goplatform.JSONError(c, http.StatusNotFound, "survey not found")
		}
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, model)
}

func (h *SurveyHandler) List(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")

	surveys, err := h.surveyService.ListSurveys(c.Request().Context(), brandID, appID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
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
		return goplatform.JSONError(c, http.StatusBadRequest, "invalid json payload")
	}

	if codeParam := c.Param("code"); codeParam != "" {
		req.Code = codeParam
	}

	if req.BrandID == "" || req.ApplicationID == "" || req.Code == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "brand_id, application_id, and code are required")
	}

	if err := h.surveyService.SaveSurvey(c.Request().Context(), req.BrandID, req.ApplicationID, req.Code, req.Title, req.Status, string(req.Schema)); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"code":    req.Code,
		"status":  entity.NormalizeSurveyStatus(req.Status), // what SaveSurvey stored
	})
}

func (h *SurveyHandler) Evaluate(c echo.Context) error {
	code := c.Param("code")
	var req domain.SurveyEvaluateRequest
	if err := c.Bind(&req); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "invalid request body")
	}

	out, err := h.surveyService.Evaluate(c.Request().Context(), code, req)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return goplatform.JSONError(c, http.StatusNotFound, "survey not found")
		}
		var verrs *domain.ValidationErrors
		if errors.As(err, &verrs) {
			return goplatform.JSONValidationError(c, http.StatusBadRequest, verrs.Errors)
		}
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
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
			return goplatform.JSONError(c, http.StatusBadRequest, "invalid data field: "+err.Error())
		}
	}
	if raw := c.FormValue("customer_conditions"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &req.CustomerConditions); err != nil {
			return goplatform.JSONError(c, http.StatusBadRequest, "invalid customer_conditions field: "+err.Error())
		}
	}

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "a photo (field \"photo\") is required")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "could not read photo: "+err.Error())
	}
	defer file.Close()

	out, err := h.surveyService.EvaluateWithPhotos(c.Request().Context(), code, req, file, fileHeader.Filename)
	if err != nil {
		if errors.Is(err, domain.ErrVisionUnavailable) {
			return goplatform.JSONErrorCode(c, http.StatusServiceUnavailable, "VISION_UNAVAILABLE", err.Error())
		}
		if errors.Is(err, domain.ErrNotFound) {
			return goplatform.JSONError(c, http.StatusNotFound, "survey not found")
		}
		var verrs *domain.ValidationErrors
		if errors.As(err, &verrs) {
			return goplatform.JSONValidationError(c, http.StatusBadRequest, verrs.Errors)
		}
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, out)
}
