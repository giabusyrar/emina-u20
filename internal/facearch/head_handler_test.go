package facearch_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/headclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
)

type fakeFitter struct {
	parts  []headclient.Part
	status int
	ct     string
	body   []byte
	err    error
}

func (f *fakeFitter) Head(_ context.Context, p []headclient.Part) (int, string, []byte, error) {
	f.parts = p
	return f.status, f.ct, f.body, f.err
}

type fakeProfiles struct{ err error }

func (f fakeProfiles) Active(context.Context, string, string) (*profile.FaceArchitectureProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &profile.FaceArchitectureProfile{ID: "p1"}, nil
}

func headRequest(t *testing.T, parts map[string][]byte) (*http.Request, *httptest.ResponseRecorder) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for name, data := range parts {
		fw, err := w.CreateFormFile(name, name+".jpg")
		require.NoError(t, err)
		_, _ = fw.Write(data)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vision/face-architecture/b/a/head", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, httptest.NewRecorder()
}

func serveHead(h *facearch.HeadHandler, req *http.Request, rec *httptest.ResponseRecorder) {
	e := echo.New()
	e.POST("/api/v1/vision/face-architecture/:brandId/:applicationId/head", h.Head)
	e.ServeHTTP(rec, req)
}

func TestHeadHandler_StreamsTheGLB(t *testing.T) {
	f := &fakeFitter{status: 200, ct: "model/gltf-binary", body: []byte("glTF")}
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F"), "right": []byte("R")})
	serveHead(facearch.NewHeadHandler(f, fakeProfiles{}, 1<<20), req, rec)
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "model/gltf-binary", rec.Header().Get("Content-Type"))
	assert.Equal(t, "glTF", rec.Body.String())
	names := []string{}
	for _, p := range f.parts {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"front", "right"}, names)
}

func TestHeadHandler_MissingFrontIs400(t *testing.T) {
	req, rec := headRequest(t, map[string][]byte{"left": []byte("L")})
	serveHead(facearch.NewHeadHandler(&fakeFitter{}, fakeProfiles{}, 1<<20), req, rec)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeMissingImage)
}

func TestHeadHandler_TooLargeIs413(t *testing.T) {
	req, rec := headRequest(t, map[string][]byte{"front": bytes.Repeat([]byte("x"), 2048)})
	serveHead(facearch.NewHeadHandler(&fakeFitter{}, fakeProfiles{}, 1024), req, rec)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeUploadTooLarge)
}

func TestHeadHandler_WorkerErrorPassesThrough(t *testing.T) {
	f := &fakeFitter{status: 422, ct: "application/json", body: []byte(`{"detail":{"code":"fit_rejected"}}`)}
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F")})
	serveHead(facearch.NewHeadHandler(f, fakeProfiles{}, 1<<20), req, rec)
	assert.Equal(t, 422, rec.Code)
	assert.JSONEq(t, `{"detail":{"code":"fit_rejected"}}`, rec.Body.String())
}

func TestHeadHandler_UnreachableIs502(t *testing.T) {
	f := &fakeFitter{err: headclient.ErrUnavailable}
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F")})
	serveHead(facearch.NewHeadHandler(f, fakeProfiles{}, 1<<20), req, rec)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeFaceHeadUnreachable)
}

func TestHeadHandler_TimeoutIs504(t *testing.T) {
	f := &fakeFitter{err: context.DeadlineExceeded}
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F")})
	serveHead(facearch.NewHeadHandler(f, fakeProfiles{}, 1<<20), req, rec)
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeFaceHeadTimeout)
}

// Mirrors Analyze: 409 with no_active_face_architecture_profile, and the
// worker is never called.
func TestHeadHandler_NoActiveProfileIs409(t *testing.T) {
	f := &fakeFitter{}
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F")})
	serveHead(facearch.NewHeadHandler(f, fakeProfiles{err: profile.ErrNoActiveProfile}, 1<<20), req, rec)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeNoActiveProfile)
	assert.Nil(t, f.parts)
}

func TestHeadHandler_ProfileLookupFailureIs500(t *testing.T) {
	req, rec := headRequest(t, map[string][]byte{"front": []byte("F")})
	serveHead(facearch.NewHeadHandler(&fakeFitter{}, fakeProfiles{err: assert.AnError}, 1<<20), req, rec)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), facearch.CodeProfileLookupFailed)
}
