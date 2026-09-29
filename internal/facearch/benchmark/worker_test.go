package benchmark_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
