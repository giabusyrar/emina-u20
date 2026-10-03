package measureclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/facearchtest"
	mc "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

func TestMeasure_SendsImageWithLandmarksAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/face-measure", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, "true", r.FormValue("landmarks"))
		f, hdr, err := r.FormFile("image")
		require.NoError(t, err)
		data, _ := io.ReadAll(f)
		assert.Equal(t, []byte("jpeg-bytes"), data)
		assert.Equal(t, "face.jpg", hdr.Filename)
		_, _ = w.Write(facearchtest.FixtureFaceJSON())
	}))
	defer srv.Close()

	resp, err := mc.New(srv.URL+"/", 5*time.Second).Measure(context.Background(), []byte("jpeg-bytes"), "face.jpg")
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", resp.CatalogueVersion)
	assert.Equal(t, "face_landmarker", resp.Model.Name)
	assert.Equal(t, []string{"roll", "yaw", "pitch", "iod_px"}, resp.Quality.GatesPassed)
	assert.Contains(t, resp.Quality.Warnings, "face_partly_out_of_frame")
	assert.Equal(t, "occluded", resp.Regions["jaw"])
	assert.Len(t, resp.Measurements, 51)
}

func TestMeasure_PassesThroughClientAndAvailabilityErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusServiceUnavailable} {
		body := `{"detail":{"code":"quality_gates_failed","failedGates":[{"gate":"roll","value":20.0,"limit":8.0,"passed":false}]}}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		_, err := mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
		srv.Close()
		var up *mc.UpstreamError
		require.True(t, errors.As(err, &up), "status %d", status)
		assert.Equal(t, status, up.Status)
		assert.JSONEq(t, body, string(up.Body))
	}
}

func TestMeasure_OtherFailuresAreUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, err := mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
	srv.Close()
	assert.ErrorIs(t, err, mc.ErrUnavailable)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	_, err = mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
	srv.Close()
	assert.ErrorIs(t, err, mc.ErrUnavailable)

	_, err = mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg") // server closed
	assert.ErrorIs(t, err, mc.ErrUnavailable)
}

func TestCatalogue_Decodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/face-measure/catalogue", r.URL.Path)
		_, _ = w.Write(facearchtest.CatalogueJSON())
	}))
	defer srv.Close()

	cat, err := mc.New(srv.URL, 5*time.Second).Catalogue(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", cat.CatalogueVersion)
	e, ok := cat.Entry("eyelid_type")
	require.True(t, ok)
	assert.False(t, e.Supported)
	e, ok = cat.Entry("width_length_ratio")
	require.True(t, ok)
	assert.Equal(t, []string{"forehead", "jaw", "contour_right", "contour_left"}, e.Regions)
}
