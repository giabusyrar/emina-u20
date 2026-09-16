package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/pkg/match/entity"
	"github.com/experience-gateway/apps/core-engine/pkg/match/repository"
	"github.com/experience-gateway/apps/core-engine/pkg/match/service"
)

func newTestMatchHandler() *MatchHandler {
	repo := repository.NewMatchRepo(nil)
	svc := service.NewMatchService(repo)
	return NewMatchHandler(svc)
}

func TestCreateAndListShades(t *testing.T) {
	h := newTestMatchHandler()
	e := echo.New()

	body, _ := json.Marshal(map[string]string{
		"productId": "prod-1",
		"name":      "Ruby Red",
		"hexColor":  "#C41E3A",
		"region":    "lip",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/matching/shades", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.CreateShade(c); err != nil {
		t.Fatalf("CreateShade error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateShade status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/matching/shades?product_id=prod-1", nil)
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)
	if err := h.ListShades(listCtx); err != nil {
		t.Fatalf("ListShades error: %v", err)
	}

	var resp struct {
		Success bool           `json:"success"`
		Shades  []entity.Shade `json:"shades"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode ListShades response: %v", err)
	}
	if !resp.Success || len(resp.Shades) != 1 {
		t.Fatalf("ListShades response = %s, want one shade", listRec.Body.String())
	}
}

func TestExtractionCallback_UpdatesStatusAndAsset(t *testing.T) {
	h := newTestMatchHandler()
	e := echo.New()

	createBody, _ := json.Marshal(map[string]string{
		"productId": "prod-1", "name": "Ruby Red", "hexColor": "#C41E3A", "region": "lip",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/matching/shades", bytes.NewReader(createBody))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	_ = h.CreateShade(createCtx)

	var createResp struct {
		Shade entity.Shade `json:"shade"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	shadeID := createResp.Shade.ID

	cbBody, _ := json.Marshal(map[string]interface{}{
		"status":       "ready",
		"colorMapUrl":  "s3://bucket/shade/color.png",
		"alphaMapUrl":  "s3://bucket/shade/alpha.png",
		"finishMapUrl": "s3://bucket/shade/finish.png",
	})
	cbReq := httptest.NewRequest(http.MethodPost, "/api/matching/shades/"+shadeID+"/extraction-callback", bytes.NewReader(cbBody))
	cbReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	cbRec := httptest.NewRecorder()
	cbCtx := e.NewContext(cbReq, cbRec)
	cbCtx.SetParamNames("id")
	cbCtx.SetParamValues(shadeID)

	if err := h.ExtractionCallback(cbCtx); err != nil {
		t.Fatalf("ExtractionCallback error: %v", err)
	}
	if cbRec.Code != http.StatusOK {
		t.Fatalf("ExtractionCallback status = %d, body=%s", cbRec.Code, cbRec.Body.String())
	}

	shades, _ := h.svc.ListShades(createCtx.Request().Context(), "prod-1")
	if len(shades) != 1 || shades[0].ExtractionStatus != "ready" {
		t.Fatalf("after callback, shades = %+v, want ExtractionStatus=ready", shades)
	}
	if shades[0].AssetID == nil {
		t.Fatal("after callback, AssetID should be set")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/matching/shade-assets/"+*shades[0].AssetID, nil)
	getRec := httptest.NewRecorder()
	getCtx := e.NewContext(getReq, getRec)
	getCtx.SetParamNames("id")
	getCtx.SetParamValues(*shades[0].AssetID)
	if err := h.GetShadeAsset(getCtx); err != nil {
		t.Fatalf("GetShadeAsset error: %v", err)
	}
	var assetResp struct {
		Success bool              `json:"success"`
		Asset   entity.ShadeAsset `json:"asset"`
	}
	_ = json.Unmarshal(getRec.Body.Bytes(), &assetResp)
	if !assetResp.Success || assetResp.Asset.ColorMapURL != "s3://bucket/shade/color.png" {
		t.Fatalf("GetShadeAsset response = %s, want matching ColorMapURL", getRec.Body.String())
	}
}

func TestCreateShade_TriggersExtractionCall(t *testing.T) {
	var receivedBody map[string]interface{}
	extractionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer extractionServer.Close()
	t.Setenv("TRYON_ENGINE_URL", extractionServer.URL)

	h := newTestMatchHandler()
	e := echo.New()

	body, _ := json.Marshal(map[string]string{
		"productId":         "prod-1",
		"name":              "Ruby Red",
		"hexColor":          "#C41E3A",
		"region":            "lip",
		"referencePhotoUrl": "https://example.com/face.jpg",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/matching/shades", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.CreateShade(c); err != nil {
		t.Fatalf("CreateShade error: %v", err)
	}

	if receivedBody == nil {
		t.Fatal("extraction endpoint was not called")
	}
	if receivedBody["hex_color"] != "#C41E3A" {
		t.Errorf("extraction request hex_color = %v, want #C41E3A", receivedBody["hex_color"])
	}
}
