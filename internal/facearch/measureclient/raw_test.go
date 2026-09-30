package measureclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestMeasureRaw_SendsWhatMeasureSendsAndReturnsBodiesVerbatim(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, hdr, err := r.FormFile("image")
		require.NoError(t, err)
		data, _ := io.ReadAll(f)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.FormValue("landmarks")+" "+hdr.Filename+" "+string(data))
		if string(data) == "bad" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream said no"))
			return
		}
		_, _ = w.Write(facearchtest.FixtureFaceJSON())
	}))
	defer srv.Close()
	c := mc.New(srv.URL+"/", 5*time.Second)

	_, err := c.Measure(context.Background(), []byte("jpeg"), "f.jpg")
	require.NoError(t, err)
	status, body, err := c.MeasureRaw(context.Background(), []byte("jpeg"), "f.jpg")
	require.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.Equal(t, facearchtest.FixtureFaceJSON(), body)
	require.Len(t, seen, 2)
	assert.Equal(t, seen[0], seen[1], "one request builder")

	status, body, err = c.MeasureRaw(context.Background(), []byte("bad"), "f.jpg")
	require.NoError(t, err, "a non-2xx answer is not a transport failure")
	assert.Equal(t, 502, status)
	assert.Equal(t, []byte("upstream said no"), body)
}

func TestMeasureRaw_TransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, _, err := mc.New(url, time.Second).MeasureRaw(context.Background(), []byte("x"), "f.jpg")
	assert.True(t, errors.Is(err, mc.ErrUnavailable))
	// The transport error stays reachable, so a caller can sanitise its
	// URL as a *url.Error instead of rewriting the message text.
	var ue *neturl.Error
	assert.True(t, errors.As(err, &ue), "the *url.Error is wrapped, not flattened")
}

func TestCatalogueRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("x") == "" && r.Header.Get("X-Bad") == "" {
			_, _ = w.Write(facearchtest.CatalogueJSON())
		}
	}))
	defer srv.Close()
	raw, err := mc.New(srv.URL, time.Second).CatalogueRaw(context.Background())
	require.NoError(t, err)
	assert.Equal(t, facearchtest.CatalogueJSON(), raw)

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	_, err = mc.New(bad.URL, time.Second).CatalogueRaw(context.Background())
	assert.ErrorIs(t, err, mc.ErrUnavailable)
}

func TestNeverFollowsRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := mc.New(srv.URL, time.Second)

	status, _, err := c.MeasureRaw(context.Background(), []byte("x"), "f.jpg")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTemporaryRedirect, status)
	_, err = c.Measure(context.Background(), []byte("x"), "f.jpg")
	assert.ErrorIs(t, err, mc.ErrUnavailable)
	assert.False(t, hit, "the redirect target received nothing")
}
