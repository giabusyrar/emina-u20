package facearch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

type fakeMeasurer struct {
	resp  *measureclient.Response
	err   error
	calls int
	got   []byte
}

func (f *fakeMeasurer) Measure(_ context.Context, image []byte, _ string) (*measureclient.Response, error) {
	f.calls++
	f.got = image
	return f.resp, f.err
}

type failingFinder struct{}

func (failingFinder) Active(context.Context, string, string) (*profile.FaceArchitectureProfile, error) {
	return nil, errors.New("connection refused")
}

const route = "/api/v1/vision/face-architecture/:brandId/:applicationId"

func serve(t *testing.T, h *facearch.Handler, withImage bool) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if withImage {
		part, err := w.CreateFormFile("image", "face.jpg")
		require.NoError(t, err)
		_, _ = part.Write([]byte("jpeg-bytes"))
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vision/face-architecture/wardah/app-1", body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.POST(route, h.Analyze)
	e.ServeHTTP(rec, req)
	return rec
}

func storeWithTestDoc(t *testing.T) *profile.MemoryStore {
	t.Helper()
	s := profile.NewMemoryStore()
	_, err := s.Upsert(context.Background(), profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "test", Document: []byte(testDoc)})
	require.NoError(t, err)
	return s
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Detail struct {
			Code string `json:"code"`
		} `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b), rec.Body.String())
	return b.Detail.Code
}

func TestAnalyze_OK(t *testing.T) {
	m := &fakeMeasurer{resp: facearchtest.FixtureFace()}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []byte("jpeg-bytes"), m.got)

	var out facearch.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "test", out.Provenance.Profile.Code)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	assert.Equal(t, "Broad", out.Traits["noseWidth"].Label)
}

func TestAnalyze_MissingImage(t *testing.T) {
	m := &fakeMeasurer{}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), false)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, facearch.CodeMissingImage, code(t, rec))
	assert.Zero(t, m.calls)
}

func TestAnalyze_NoActiveProfileIs409WithoutCallingWorker(t *testing.T) {
	m := &fakeMeasurer{resp: facearchtest.FixtureFace()}
	rec := serve(t, facearch.NewHandler(m, profile.NewMemoryStore()), true)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, facearch.CodeNoActiveProfile, code(t, rec))
	assert.Contains(t, rec.Body.String(), `"brandId":"wardah"`)
	assert.Zero(t, m.calls)
}

func TestAnalyze_PassesWorkerErrorsThrough(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusServiceUnavailable} {
		body := fmt.Sprintf(`{"detail":{"code":"from_worker_%d"}}`, status)
		m := &fakeMeasurer{err: &measureclient.UpstreamError{Status: status, Body: []byte(body)}}
		rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
		assert.Equal(t, status, rec.Code)
		assert.JSONEq(t, body, rec.Body.String())
	}
}

func TestAnalyze_UnreachableWorkerIs503(t *testing.T) {
	m := &fakeMeasurer{err: fmt.Errorf("%w: dial tcp: refused", measureclient.ErrUnavailable)}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, facearch.CodeFaceMeasureUnreachable, code(t, rec))
}

func TestAnalyze_ProfileFailures(t *testing.T) {
	rec := serve(t, facearch.NewHandler(&fakeMeasurer{}, failingFinder{}), true)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, facearch.CodeProfileLookupFailed, code(t, rec))
	assert.NotContains(t, rec.Body.String(), "connection refused", "database errors are logged, not returned")

	s := profile.NewMemoryStore()
	s.Put(profile.FaceArchitectureProfile{BrandID: "*", ApplicationID: "*", Code: "bad", Version: 1, Status: "ACTIVE", Document: `{"schemaVersion": 9}`})
	m := &fakeMeasurer{}
	rec = serve(t, facearch.NewHandler(m, s), true)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, facearch.CodeProfileDocumentInvalid, code(t, rec))
	assert.Zero(t, m.calls)
}
