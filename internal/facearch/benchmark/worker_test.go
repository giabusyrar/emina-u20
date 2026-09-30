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

// An apostrophe is legal in userinfo and in a query, and net/http keeps it
// in the URL it quotes; the secret after it must not survive.
func TestHTTPWorkerErrorsCarryNoSecretsAfterAnApostrophe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()
	raw := strings.Replace(srv.URL, "http://", "http://o'brien:hun'ter2@", 1) + "?token=ab'cdsecret"
	w := benchmark.NewHTTPWorker(raw, 5*time.Second)

	_, _, err := w.Measure(context.Background(), []byte("x"), "image.jpg")
	require.Error(t, err)
	for _, leak := range []string{"brien", "ter2", "token", "cdsecret"} {
		assert.NotContains(t, err.Error(), leak)
	}
}

// A worker URL without scheme://host is refused before anything is sent:
// Go would read "user" as the scheme, the password would never be seen as
// userinfo, and every error would print it.
func TestCheckWorkerURL(t *testing.T) {
	for _, bad := range []string{"", "not a url", "user:hunter2@ai-worker.test:8088", "ai-worker.test:8088", "ftp://ai-worker.test"} {
		err := benchmark.CheckWorkerURL(bad)
		require.Error(t, err, bad)
		assert.NotContains(t, err.Error(), "hunter2", "the refusal does not echo the URL")
	}
	for _, good := range []string{"http://ai-worker.test:8088", "https://someuser:pw@ai-worker.test/base"} {
		assert.NoError(t, benchmark.CheckWorkerURL(good), good)
	}
}
