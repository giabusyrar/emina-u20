package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/adapters/workerclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
)

type fakeStore struct{ c []Candidate }

func (f fakeStore) ListCandidates(context.Context) ([]Candidate, error) { return f.c, nil }
func (f fakeStore) GetCandidates(_ context.Context, ids []string) ([]Candidate, error) {
	var out []Candidate
	for _, c := range f.c {
		for _, id := range ids {
			if c.ShadeID == id {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

type fakeWorker struct {
	layers   []workerclient.Layer
	qcFailed []string
}

func (f *fakeWorker) Analyze(context.Context, []byte, string, bool, bool) (*workerclient.AnalyzeResult, error) {
	r := &workerclient.AnalyzeResult{}
	skin := domain.Lab{74.8, 8.55, 11.31}
	r.Measurement.SkinLab = &skin
	r.Measurement.LPerSite = map[string]float64{"forehead": 77.8, "jaw_r": 87.3, "cheek_l": 60.6}
	r.Munsell.Skin = domain.Munsell{V: 7.41, C: 2.84, ChromaSource: "approximation"}
	r.Quality.QCFailed = f.qcFailed
	return r, nil
}
func (f *fakeWorker) Render(_ context.Context, _ []byte, _ string, l []workerclient.Layer) ([]byte, error) {
	f.layers = l
	return []byte("png"), nil
}

func newSvc(t *testing.T, debug bool) (*Service, *fakeWorker) {
	cfg, err := domain.LoadConfig(filepath.Join("..", "..", "..", "config", "wcpa.json"))
	if err != nil {
		t.Fatal(err)
	}
	w := &fakeWorker{}
	store := fakeStore{c: []Candidate{
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "lip1", ProductID: "p1", ProductName: "B Lip", Category: "lip", HexColor: "#C33370", Lab: domain.Lab{70, 10, 12}, V: 8.4, C: 4.3, ColourSource: "swatch"}, Alpha: 0.75},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "lip2", ProductID: "p1", ProductName: "B Lip", Category: "lip", HexColor: "#C33371", Lab: domain.Lab{70, 10, 12}, V: 8.4, C: 4.3}, Alpha: 0.75},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "lip3", ProductID: "p2", ProductName: "A Lip", Category: "lip", HexColor: "#401010", Lab: domain.Lab{20, 25, 10}, V: 2.0, C: 6.5, ColourSource: "estimate"}, Alpha: 0.8},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "lip4", ProductID: "p2", ProductName: "A Lip", Category: "lip", HexColor: "#E0A0A0", Lab: domain.Lab{75, 20, 10}, V: 5.0, C: 4.0, ColourSource: "estimate"}, Alpha: 0.8},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "brow1", ProductID: "p3", ProductName: "Brow", Category: "brow", HexColor: "#88765E", Lab: domain.Lab{50, 2, 12}, V: 4.8, C: 2.4}, Alpha: 0.6, Mode: "tint"},
		{ShadeCandidate: domain.ShadeCandidate{ShadeID: "masc1", ProductID: "p4", ProductName: "Mascara", Category: "mascara", HexColor: "#000000", Lab: domain.Lab{0, 0, 0}, V: 0, C: 0}, Alpha: 0.8, Mode: "volumizing"},
	}}
	return New(cfg, store, w, debug), w
}

func TestAnalyzeHidesRawColourUnlessDebugEnabled(t *testing.T) {
	s, _ := newSvc(t, false)
	out, err := s.Analyze(context.Background(), []byte("x"), "a.jpg", false, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Debug != nil {
		t.Fatal("debug must stay nil when the deployment has not enabled it")
	}
	if out.Quadrant.Code != "HV_LC" || !out.Quadrant.Provisional || out.Quadrant.DisplayName != nil {
		t.Fatalf("quadrant %+v", out.Quadrant)
	}
	if out.FoundationBand != "very light" || out.Labels.Undertone != "sejuk (merah)" {
		t.Fatalf("labels %+v band %s", out.Labels, out.FoundationBand)
	}
	want := map[string]bool{"uneven_skin_tone": true, "chroma_approximated": true}
	for _, f := range out.Flags {
		delete(want, f)
	}
	if len(want) != 0 {
		t.Fatalf("missing flags %v in %v", want, out.Flags)
	}
	if len(out.Recommendations["lip"]) != 2 {
		t.Fatalf("lip recs %+v", out.Recommendations)
	}
	if _, ok := out.Recommendations["brow"]; ok {
		t.Fatalf("brow is not rated, got recommendations %+v", out.Recommendations["brow"])
	}
	s2, _ := newSvc(t, true)
	out2, _ := s2.Analyze(context.Background(), []byte("x"), "a.jpg", false, true, true)
	if out2.Debug == nil {
		t.Fatal("debug expected when enabled and requested")
	}
}

func TestCatalogHasEveryShade(t *testing.T) {
	s, _ := newSvc(t, false)
	out, err := s.Analyze(context.Background(), []byte("x"), "a.jpg", false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	lips := out.Catalog["lip"]
	ids := []string{}
	for _, l := range lips {
		ids = append(ids, l.ShadeID+":"+l.Status)
	}
	// products by name (A before B), each light to dark; every shade is
	// there, kept ones with their status, the others with none
	want := []string{"lip4:", "lip3:", "lip1:same_quadrant", "lip2:same_quadrant"}
	if len(ids) != len(want) {
		t.Fatalf("lip catalog %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("lip catalog %v, want %v", ids, want)
		}
	}
	if lips[0].ColourSource != "estimate" || lips[0].HueName == "" {
		t.Fatalf("lip4 %+v", lips[0])
	}
	m := out.Catalog["mascara"]
	if len(m) != 1 || m[0].Status != "" || m[0].Mode != "volumizing" || m[0].HueName != "" {
		t.Fatalf("mascara must be offered unrated, with its type and no hue name: %+v", m)
	}
	if b := out.Catalog["brow"]; len(b) != 1 || b[0].Status != "" {
		t.Fatalf("brow %+v", b)
	}

	plain, err := s.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Catalog["lip"]) != 4 || plain.ConfigVersion == "" {
		t.Fatalf("catalog %+v", plain)
	}
	for cat, list := range plain.Catalog {
		for _, sh := range list {
			if sh.Status != "" {
				t.Fatalf("%s %s: no analysis, no status (got %q)", cat, sh.ShadeID, sh.Status)
			}
		}
	}
}

func TestTryOnUsesCatalogValues(t *testing.T) {
	s, w := newSvc(t, false)
	if _, err := s.TryOn(context.Background(), []byte("x"), "a.jpg", []string{"lip1", "brow1"}); err != nil {
		t.Fatal(err)
	}
	if len(w.layers) != 2 || w.layers[0].HexColor != "#C33370" || w.layers[1].Mode != "tint" {
		t.Fatalf("layers %+v", w.layers)
	}
	if _, err := s.TryOn(context.Background(), []byte("x"), "a.jpg", []string{"nope"}); !errors.Is(err, domain.ErrShadeNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.TryOn(context.Background(), []byte("x"), "a.jpg", []string{"lip1", "lip2"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("two lip shades: got %v", err)
	}
}
