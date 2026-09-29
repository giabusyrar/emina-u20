package benchmark_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestHTTPWorkerSendsWhatTheEndpointSends(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case mc.CataloguePath:
			_, _ = w.Write(facearchtest.CatalogueJSON())
		case mc.MeasurePath:
			require.NoError(t, r.ParseMultipartForm(1<<20))
			assert.Equal(t, "true", r.FormValue("landmarks"))
			f, hdr, err := r.FormFile("image")
			require.NoError(t, err)
			data, _ := io.ReadAll(f)
			assert.Equal(t, "image.jpg", hdr.Filename)
			switch string(data) {
			case "ok":
				_, _ = w.Write(facearchtest.FixtureFaceJSON())
			case "gate":
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write(gateBody)
			default:
				w.WriteHeader(http.StatusBadGateway)
			}
		}
	}))
	defer srv.Close()
	w := benchmark.NewHTTPWorker(srv.URL+"/", time.Second)
	assert.Equal(t, srv.URL, w.BaseURL())

	cat, err := w.Catalogue(context.Background())
	require.NoError(t, err)
	assert.Equal(t, facearchtest.CatalogueJSON(), cat)

	status, body, err := w.Measure(context.Background(), []byte("ok"), "image.jpg")
	require.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.Equal(t, facearchtest.FixtureFaceJSON(), body, "the exact bytes, for the cache")

	status, body, err = w.Measure(context.Background(), []byte("gate"), "image.jpg")
	require.NoError(t, err)
	assert.Equal(t, 422, status)
	assert.Equal(t, gateBody, body)

	_, _, err = w.Measure(context.Background(), []byte("other"), "image.jpg")
	assert.ErrorIs(t, err, mc.ErrUnavailable)
}

func TestSafeURLDropsUserinfoQueryAndFragment(t *testing.T) {
	for raw, want := range map[string]string{
		"http://someuser:hunter2@ai-worker.test:8000/base?token=secret#frag": "http://ai-worker.test:8000/base",
		"http://someuser@ai-worker.test?token=secret/api/v1/face-measure":    "http://ai-worker.test",
		"https://ai-worker.test/": "https://ai-worker.test/",
		"not a url":               benchmark.UnparseableURL,
		"http://[::1":             benchmark.UnparseableURL,
	} {
		assert.Equal(t, want, benchmark.SafeURL(raw), raw)
	}
}

// A transport failure from the real client is a *url.Error rendered into
// measureclient's message; the worker's error carries no user, password
// or token, and still says ErrUnavailable.
func TestHTTPWorkerErrorsCarryNoURLSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()
	raw := strings.Replace(srv.URL, "http://", "http://someuser:hunter2@", 1) + "?token=secret"
	w := benchmark.NewHTTPWorker(raw, 5*time.Second)

	_, catErr := w.Catalogue(context.Background())
	_, _, measureErr := w.Measure(context.Background(), []byte("x"), "image.jpg")
	for _, err := range []error{catErr, measureErr} {
		require.Error(t, err)
		assert.ErrorIs(t, err, mc.ErrUnavailable)
		for _, leak := range []string{"someuser", "hunter2", "token", "secret"} {
			assert.NotContains(t, err.Error(), leak)
		}
		assert.Contains(t, err.Error(), strings.TrimPrefix(srv.URL, "http://"), "the host is still named")
	}
}
