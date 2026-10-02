package conversationflow

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/packages/goplatform"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount registers the flow routes on g.
func (h *Handler) Mount(g *echo.Group) {
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/active", h.Active)
	g.PUT("/:id", h.Update)
}

// List handles GET ?brand_id=&application_id=.
func (h *Handler) List(c echo.Context) error {
	brandID, appID := c.QueryParam("brand_id"), c.QueryParam("application_id")
	if brandID == "" || appID == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "brand_id and application_id query parameters are required")
	}
	flows, err := h.svc.List(c.Request().Context(), brandID, appID)
	if err != nil {
		return fail(c, err)
	}
	if flows == nil {
		flows = []Flow{}
	}
	return c.JSON(http.StatusOK, map[string]any{"flows": flows})
}

// Active handles GET /active?brand_id=&application_id=&survey_code=.
func (h *Handler) Active(c echo.Context) error {
	brandID, appID, code := c.QueryParam("brand_id"), c.QueryParam("application_id"), c.QueryParam("survey_code")
	if brandID == "" || appID == "" || code == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "brand_id, application_id and survey_code query parameters are required")
	}
	f, err := h.svc.Active(c.Request().Context(), brandID, appID, code)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, f)
}

// Create handles POST with a Flow body.
func (h *Handler) Create(c echo.Context) error {
	var f Flow
	if err := c.Bind(&f); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	warnings, err := h.svc.Create(c.Request().Context(), &f)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"flow": f, "warnings": warnings})
}

// Update handles PUT /:id with a Flow body.
func (h *Handler) Update(c echo.Context) error {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "id must be a UUID")
	}
	var f Flow
	if err := c.Bind(&f); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	warnings, err := h.svc.Update(c.Request().Context(), id, &f)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"flow": f, "warnings": warnings})
}

func fail(c echo.Context, err error) error {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		return goplatform.JSONValidationError(c, http.StatusBadRequest, verr.Errors)
	case errors.Is(err, ErrNotFound):
		return goplatform.JSONError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUnknownBrand):
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}
	return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
}
