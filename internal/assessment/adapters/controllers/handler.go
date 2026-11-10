package controllers

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/service"
	goplatform "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/platform"
)

type Handler struct {
	store    service.Store
	maxLimit int
}

// NewHandler serves stored assessments; maxLimit (ASSESSMENT_LIST_MAX_LIMIT)
// caps one response and is the size when no limit is given.
func NewHandler(store service.Store, maxLimit int) *Handler {
	return &Handler{store: store, maxLimit: maxLimit}
}

// Mount registers GET /customers/:customerId and GET /history on g.
func (h *Handler) Mount(g *echo.Group) {
	g.GET("/customers/:customerId", h.ListCustomer)
	g.GET("/history", h.History)
}

// ListCustomer handles GET /customers/:customerId?brand_id=&application_id=&limit=.
// Customer ids are supplied by each brand, so the brand scope is required.
func (h *Handler) ListCustomer(c echo.Context) error {
	return h.list(c, c.Param("customerId"))
}

// History handles GET /history?brand_id=&application_id=&limit=.
func (h *Handler) History(c echo.Context) error {
	return h.list(c, "")
}

func (h *Handler) list(c echo.Context, customerID string) error {
	f := service.Filter{
		BrandID:       c.QueryParam("brand_id"),
		ApplicationID: c.QueryParam("application_id"),
		CustomerID:    customerID,
		Limit:         h.maxLimit,
	}
	if f.BrandID == "" || f.ApplicationID == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "brand_id and application_id query parameters are required")
	}
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return goplatform.JSONError(c, http.StatusBadRequest, "limit must be a positive integer")
		}
		if n < f.Limit {
			f.Limit = n
		}
	}
	items, err := h.store.List(c.Request().Context(), f)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	if items == nil {
		items = []entity.Record{}
	}
	return c.JSON(http.StatusOK, map[string]any{"assessments": items, "limit": f.Limit})
}
