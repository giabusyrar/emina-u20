package controllers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/pkg/score/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/score/entity"
	"github.com/experience-gateway/apps/core-engine/pkg/score/service"
)

type ScoreHandler struct {
	svc *service.ScoreService
}

func NewScoreHandler(svc *service.ScoreService) *ScoreHandler {
	return &ScoreHandler{svc: svc}
}

// Evaluate handles POST /api/scoring/evaluate
func (h *ScoreHandler) Evaluate(c echo.Context) error {
	var req domain.ScoreModuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON payload: " + err.Error(),
		})
	}

	resp, err := h.svc.Evaluate(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

func queryParamFirst(c echo.Context, keys ...string) string {
	for _, k := range keys {
		if val := c.QueryParam(k); val != "" {
			return val
		}
	}
	return ""
}

// ListRulesets handles GET /api/scoring/rulesets
func (h *ScoreHandler) ListRulesets(c echo.Context) error {
	brandID := queryParamFirst(c, "brand_id", "brandId")
	appID := queryParamFirst(c, "application_id", "applicationId", "appId")

	rulesets, err := h.svc.ListRulesets(c.Request().Context(), brandID, appID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":  true,
		"rulesets": rulesets,
	})
}

// GetActiveRuleset handles GET /api/scoring/rulesets/active
func (h *ScoreHandler) GetActiveRuleset(c echo.Context) error {
	brandID := queryParamFirst(c, "brand_id", "brandId")
	appID := queryParamFirst(c, "application_id", "applicationId", "appId")
	code := queryParamFirst(c, "code")

	ruleset, err := h.svc.GetActiveRuleset(c.Request().Context(), brandID, appID, code)
	if err != nil || ruleset == nil {
		msg := "active ruleset not found"
		if err != nil {
			msg = err.Error()
		}
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   msg,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"ruleset": ruleset,
	})
}

// GetRuleset handles GET /api/scoring/rulesets/:id
func (h *ScoreHandler) GetRuleset(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		id = c.QueryParam("id")
	}
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Ruleset ID is required",
		})
	}

	ruleset, err := h.svc.GetRuleset(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"ruleset": ruleset,
	})
}

// CreateRuleset handles POST /api/scoring/rulesets
func (h *ScoreHandler) CreateRuleset(c echo.Context) error {
	var item entity.ScoreRuleset
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid ruleset payload: " + err.Error(),
		})
	}

	if item.Code == "" || item.Title == "" || item.Schema == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "code, title, and schema are required fields",
		})
	}

	if err := h.svc.SaveRuleset(c.Request().Context(), &item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success": true,
		"ruleset": item,
	})
}

// UpdateRuleset handles PUT /api/scoring/rulesets/:id
func (h *ScoreHandler) UpdateRuleset(c echo.Context) error {
	id := c.Param("id")
	var item entity.ScoreRuleset
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid ruleset payload: " + err.Error(),
		})
	}

	if id != "" {
		item.ID = id
	}

	if err := h.svc.SaveRuleset(c.Request().Context(), &item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"ruleset": item,
	})
}

// DeleteRuleset handles DELETE /api/scoring/rulesets/:id
func (h *ScoreHandler) DeleteRuleset(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		id = c.QueryParam("id")
	}
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Ruleset ID is required",
		})
	}

	if err := h.svc.DeleteRuleset(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Ruleset deleted successfully",
	})
}

// Simulate handles POST /api/scoring/simulate
func (h *ScoreHandler) Simulate(c echo.Context) error {
	var req entity.RulesetSimulationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid simulation payload: " + err.Error(),
		})
	}

	resp, err := h.svc.Simulate(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}
