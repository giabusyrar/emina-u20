package controllers

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/core-engine/internal/match/adapters/objectstore"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/repository"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
)

type fakePresigner struct{ err error }

func (f fakePresigner) Presign(_ context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "https://signed.example/" + key + "?sig=1", nil
}

func newTestMatchHandlerWith(p objectstore.Presigner) *MatchHandler {
	repo := repository.NewMatchRepo(nil)
	return NewMatchHandler(service.NewMatchService(repo, ""), p)
}

func newTestMatchHandler() *MatchHandler { return newTestMatchHandlerWith(fakePresigner{}) }

func getShadeAsset(t *testing.T, h *MatchHandler, id string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/matching/shade-assets/"+id, nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := h.GetShadeAsset(c); err != nil {
		t.Fatalf("GetShadeAsset error: %v", err)
	}
	return rec
}

func saveAsset(t *testing.T, h *MatchHandler, a entity.ShadeAsset) string {
	t.Helper()
	saved, err := h.svc.SaveShadeAsset(context.Background(), a)
	if err != nil {
		t.Fatalf("SaveShadeAsset: %v", err)
	}
	return saved.ID
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
		"colorMapKey":  "shades/s1/color.png",
		"alphaMapKey":  "shades/s1/alpha.png",
		"finishMapKey": "shades/s1/finish.png",
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
		Asset   map[string]string `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &assetResp); err != nil {
		t.Fatalf("decode: %v body=%s", err, getRec.Body.String())
	}
	if !assetResp.Success || assetResp.Asset["colorMapUrl"] != "https://signed.example/shades/s1/color.png?sig=1" ||
		assetResp.Asset["finishMapUrl"] != "https://signed.example/shades/s1/finish.png?sig=1" {
		t.Fatalf("GetShadeAsset response = %s, want signed URLs", getRec.Body.String())
	}
	if _, leaked := assetResp.Asset["colorMapKey"]; leaked {
		t.Fatalf("response exposes stored keys: %s", getRec.Body.String())
	}
}

func TestCreateShade_TriggersExtractionCall(t *testing.T) {
	var receivedBody map[string]interface{}
	extractionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer extractionServer.Close()

	repo := repository.NewMatchRepo(nil)
	svc := service.NewMatchService(repo, extractionServer.URL)
	h := NewMatchHandler(svc, fakePresigner{})
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

func TestExtractionCallback_ReadyWithoutKeysIs400(t *testing.T) {
	h := newTestMatchHandler()
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"status": "ready", "colorMapUrl": "https://old.example/c.png"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("any")
	if err := h.ExtractionCallback(c); err != nil {
		t.Fatalf("ExtractionCallback error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_UnconfiguredStorageIs503(t *testing.T) {
	h := newTestMatchHandlerWith(nil)
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("object storage not configured")) {
		t.Fatalf("status=%d body=%s, want 503 object storage not configured", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_NotFoundIs404(t *testing.T) {
	rec := getShadeAsset(t, newTestMatchHandler(), "missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetShadeAsset_RowWithoutKeysIs409(t *testing.T) {
	h := newTestMatchHandler()
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("shade-asset-keys")) {
		t.Fatalf("status=%d body=%s, want 409 naming the migration", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_SignFailureIs500WithoutEcho(t *testing.T) {
	h := newTestMatchHandlerWith(fakePresigner{err: errors.New("secret-internal-detail")})
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusInternalServerError || bytes.Contains(rec.Body.Bytes(), []byte("secret-internal-detail")) {
		t.Fatalf("status=%d body=%s, want 500 without the internal error", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_NoFinishKeyOmitsFinishMapUrl(t *testing.T) {
	h := newTestMatchHandler()
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("finishMapUrl")) {
		t.Fatalf("status=%d body=%s, want 200 without finishMapUrl", rec.Code, rec.Body.String())
	}
}

// failingDriver is a database/sql driver whose reads return one shade row and
// whose every write fails with a recognisable message, so the callback's 500
// paths can be driven without a database.
type failingDriver struct{}

const writeFailureDetail = "pq: secret-internal-detail"

func (failingDriver) Open(string) (driver.Conn, error) { return failingConn{}, nil }

type failingConn struct{}

func (failingConn) Prepare(q string) (driver.Stmt, error) { return failingStmt{}, nil }
func (failingConn) Close() error                          { return nil }
func (failingConn) Begin() (driver.Tx, error)             { return nil, errors.New(writeFailureDetail) }

type failingStmt struct{}

func (failingStmt) Close() error  { return nil }
func (failingStmt) NumInput() int { return -1 }
func (failingStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New(writeFailureDetail)
}
func (failingStmt) Query([]driver.Value) (driver.Rows, error) { return &oneShadeRows{}, nil }

type oneShadeRows struct{ done bool }

func (r *oneShadeRows) Columns() []string { return []string{"id"} }
func (r *oneShadeRows) Close() error      { return nil }
func (r *oneShadeRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = "shade-1"
	return nil
}

func TestExtractionCallback_WriteFailureIs500WithoutEcho(t *testing.T) {
	sql.Register("failing-callback-driver", failingDriver{})
	sqlDB, err := sql.Open("failing-callback-driver", "")
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewMatchHandler(service.NewMatchService(repository.NewMatchRepo(gdb), ""), fakePresigner{})

	for name, payload := range map[string]map[string]string{
		"SaveShadeAsset": {"status": "ready", "colorMapKey": "shades/1/color.png", "alphaMapKey": "shades/1/alpha.png"},
		"UpdateShade":    {"status": "failed", "reason": "bad"},
	} {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("shade-1")
		if err := h.ExtractionCallback(c); err != nil {
			t.Fatalf("%s: ExtractionCallback error: %v", name, err)
		}
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: status = %d, want 500; body=%s", name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "secret-internal-detail") {
			t.Fatalf("%s: response leaks the internal error: %s", name, rec.Body.String())
		}
	}
}
