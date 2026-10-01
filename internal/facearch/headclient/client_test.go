package headclient_test

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

	"github.com/experience-gateway/apps/core-engine/internal/facearch/headclient"
)

func TestHead_SendsPartsAndReturnsBytesVerbatim(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, headclient.Path, r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		got = map[string]string{}
		for name, fhs := range r.MultipartForm.File {
			f, _ := fhs[0].Open()
			b, _ := io.ReadAll(f)
			got[name] = string(b)
		}
		w.Header().Set("Content-Type", "model/gltf-binary")
		_, _ = w.Write([]byte("glTF-bytes"))
	}))
	defer srv.Close()
	c := headclient.New(srv.URL, 5*time.Second)
	st, ct, body, err := c.Head(context.Background(), []headclient.Part{
		{Name: "front", Filename: "f.jpg", Data: []byte("F")}, {Name: "left", Filename: "l.jpg", Data: []byte("L")}})
	require.NoError(t, err)
	assert.Equal(t, 200, st)
	assert.Equal(t, "model/gltf-binary", ct)
	assert.Equal(t, "glTF-bytes", string(body))
	assert.Equal(t, map[string]string{"front": "F", "left": "L"}, got)
}

func TestHead_PassesErrorBodiesThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"detail":{"code":"wrong_side","view":"left"}}`))
	}))
	defer srv.Close()
	st, ct, body, err := headclient.New(srv.URL, time.Second).Head(context.Background(),
		[]headclient.Part{{Name: "front", Filename: "f", Data: []byte("x")}})
	require.NoError(t, err)
	assert.Equal(t, 422, st)
	assert.Equal(t, "application/json", ct)
	assert.JSONEq(t, `{"detail":{"code":"wrong_side","view":"left"}}`, string(body))
}

func TestHead_TransportFailureIsErrUnavailable(t *testing.T) {
	_, _, _, err := headclient.New("http://127.0.0.1:1", time.Second).Head(context.Background(),
		[]headclient.Part{{Name: "front", Filename: "f", Data: []byte("x")}})
	assert.ErrorIs(t, err, headclient.ErrUnavailable)
}

func TestHead_TimeoutIsDetectable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	_, _, _, err := headclient.New(srv.URL, 50*time.Millisecond).Head(context.Background(),
		[]headclient.Part{{Name: "front", Filename: "f", Data: []byte("x")}})
	require.ErrorIs(t, err, headclient.ErrUnavailable)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "timeout must be detectable: %v", err)
}
