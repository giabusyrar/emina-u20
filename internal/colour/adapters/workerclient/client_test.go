package workerclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
)

func TestAnalyzeSendsFormAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pca/analyze" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("hijab") != "true" || r.FormValue("hairVisible") != "false" {
			t.Fatalf("fields %v", r.MultipartForm.Value)
		}
		f, _, err := r.FormFile("image")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		if string(b) != "img" {
			t.Fatalf("image %q", b)
		}
		w.Write([]byte(`{"measurement":{"skinLab":[74.8,8.55,11.31],"hairLab":null},"munsell":{"skin":{"V":7.41,"C":2.84,"chromaSource":"approximation"}},"quality":{"qcFailed":["wajah_besar"]}}`))
	}))
	defer srv.Close()

	res, err := New(srv.URL, time.Second).Analyze(context.Background(), []byte("img"), "a.jpg", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Measurement.SkinLab[0] != 74.8 || res.Measurement.HairLab != nil || res.Munsell.Skin.V != 7.41 {
		t.Fatalf("decoded %+v", res)
	}
}

func TestWorkerErrorsMapToSentinels(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"noface":  {422, `{"detail":{"code":"no_face_detected","message":"x"}}`, domain.ErrNoFace},
		"badimg":  {400, `{"detail":{"code":"invalid_image","message":"x"}}`, domain.ErrInvalidInput},
		"model":   {503, `{"detail":{"code":"model_unavailable","message":"x"}}`, domain.ErrWorkerUnavailable},
		"garbage": {500, `oops`, domain.ErrWorkerUnavailable},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		_, err := New(srv.URL, time.Second).Render(context.Background(), []byte("img"), "a.png", []Layer{{Category: "lip", HexColor: "#AA0000", Alpha: 0.8}})
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v want %v", name, err, tc.want)
		}
	}
}

func TestUnreachableWorker(t *testing.T) {
	_, err := New("http://127.0.0.1:1", 200*time.Millisecond).MunsellFromHex(context.Background(), []string{"#AA0000"})
	if !errors.Is(err, domain.ErrWorkerUnavailable) {
		t.Fatalf("got %v", err)
	}
}
