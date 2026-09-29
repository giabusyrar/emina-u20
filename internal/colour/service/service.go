// Package service is the WCPA flow: measure (ai-worker colour profile) →
// classify (config rules) → recommend (catalog) → present; and photo try-on by
// shade id.
package service

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/experience-gateway/apps/core-engine/internal/colour/adapters/workerclient"
	"github.com/experience-gateway/apps/core-engine/internal/colour/domain"
)

// Candidate is a catalog shade plus its render parameters.
type Candidate struct {
	domain.ShadeCandidate
	Alpha    float64
	Specular float64
	Mode     string
}

type Store interface {
	ListCandidates(ctx context.Context) ([]Candidate, error)
	GetCandidates(ctx context.Context, shadeIDs []string) ([]Candidate, error)
}

type Worker interface {
	Analyze(ctx context.Context, image []byte, filename string, hijab, hairVisible bool) (*workerclient.AnalyzeResult, error)
	Render(ctx context.Context, image []byte, filename string, layers []workerclient.Layer) ([]byte, error)
}

type Service struct {
	cfg          *domain.Config
	store        Store
	worker       Worker
	debugEnabled bool
}

func New(cfg *domain.Config, store Store, worker Worker, debugEnabled bool) *Service {
	return &Service{cfg: cfg, store: store, worker: worker, debugEnabled: debugEnabled}
}

// ---- response shapes (contract §3) ----------------------------------------

type QuadrantOut struct {
	Code          string  `json:"code"`
	TechnicalName string  `json:"technicalName"`
	DisplayName   *string `json:"displayName"`
	Provisional   bool    `json:"provisional"`
}

type ComplementaryOut struct {
	Codes           []string `json:"codes"`
	TemperatureRule string   `json:"temperatureRule"`
	Provisional     bool     `json:"provisional"`
}

type LabelsOut struct {
	Undertone          string   `json:"undertone"`
	Value              string   `json:"value"`
	Chroma             string   `json:"chroma"`
	SeasonEquivalents  []string `json:"seasonEquivalents"`
	SeasonsProvisional bool     `json:"seasonsProvisional"`
}

type RecommendationOut struct {
	ShadeID      string `json:"shadeId"`
	ProductID    string `json:"productId"`
	ProductName  string `json:"productName"`
	ShadeName    string `json:"shadeName"`
	HexColor     string `json:"hexColor"`
	HueName      string `json:"hueName"`
	Status       string `json:"status"`
	ColourSource string `json:"colourSource"`
}

// CatalogShadeOut is one shade of the try-on catalog. Status is
// same_quadrant or complementary for a shade the quadrant rules keep for this
// person, "" otherwise: a shade outside both, a category the rules do not
// rate (config recommendation.categories), or no analysis (GET /catalog).
// Mode is the render style (mascara type, eyeliner and eyeshadow look).
type CatalogShadeOut struct {
	ShadeID      string `json:"shadeId"`
	ProductID    string `json:"productId"`
	ProductName  string `json:"productName"`
	ShadeName    string `json:"shadeName"`
	HexColor     string `json:"hexColor"`
	HueName      string `json:"hueName"`
	Status       string `json:"status"`
	ColourSource string `json:"colourSource"`
	Mode         string `json:"mode"`
}

// CatalogResponse is GET /catalog: every shade to try, without an analysis.
type CatalogResponse struct {
	Catalog       map[string][]CatalogShadeOut `json:"catalog"`
	ConfigVersion string                       `json:"configVersion"`
}

type AnalyzeResponse struct {
	Quadrant       QuadrantOut      `json:"quadrant"`
	Complementary  ComplementaryOut `json:"complementary"`
	Labels         LabelsOut        `json:"labels"`
	FoundationBand string           `json:"foundationBand"`
	Flags          []string         `json:"flags"`
	QualityFailed  []string         `json:"qualityFailed"`
	// Recommendations is the notebook's list: per rated category, the kept
	// shades best first, at most recommendation.perCategory.
	Recommendations map[string][]RecommendationOut `json:"recommendations"`
	// Catalog is every shade of every category, with this person's status,
	// for the try-on (a category with no kept shade still has shades to try).
	Catalog       map[string][]CatalogShadeOut `json:"catalog"`
	ConfigVersion string                       `json:"configVersion"`
	Debug         map[string]interface{}       `json:"debug"`
}

// Analyze runs the whole WCPA flow for one photo.
func (s *Service) Analyze(ctx context.Context, image []byte, filename string, hijab, hairVisible, debug bool) (*AnalyzeResponse, error) {
	if len(image) == 0 {
		return nil, fmt.Errorf("%w: image is required", domain.ErrInvalidInput)
	}
	m, err := s.worker.Analyze(ctx, image, filename, hijab, hairVisible)
	if err != nil {
		return nil, err
	}
	skin := *m.Measurement.SkinLab
	v, c := m.Munsell.Skin.V, m.Munsell.Skin.C

	code, gv, gc, dist := s.cfg.PersonQuadrant(v, c)
	temp, bMinusA := s.cfg.TemperatureOf(skin)
	comp := s.cfg.Complementary.Map[code]

	var display *string
	if n := s.cfg.Presentation.DisplayNames[code]; n != "" {
		display = &n
	}

	flags := []string{}
	if lRange(m.Measurement.LPerSite) > s.cfg.Flags.UnevenSkinToneLRange {
		flags = append(flags, "uneven_skin_tone")
	}
	if m.Munsell.Skin.ChromaSource != "colour-science" {
		flags = append(flags, "chroma_approximated")
	}
	if !hairVisible || hijab {
		flags = append(flags, "hair_not_used")
	}

	shades, err := s.store.ListCandidates(ctx)
	if err != nil {
		return nil, err
	}
	byCat := map[string][]domain.ShadeCandidate{}
	for _, sh := range shades {
		if s.cfg.Rates(sh.Category) {
			byCat[sh.Category] = append(byCat[sh.Category], sh.ShadeCandidate)
		}
	}
	recs := map[string][]RecommendationOut{}
	for cat, list := range byCat {
		for _, r := range s.cfg.Recommend(code, v, c, list) {
			recs[cat] = append(recs[cat], RecommendationOut{
				ShadeID: r.ShadeID, ProductID: r.ProductID, ProductName: r.ProductName,
				ShadeName: r.ShadeName, HexColor: r.HexColor, HueName: r.HueName,
				Status: r.Status, ColourSource: r.ColourSource,
			})
		}
	}

	out := &AnalyzeResponse{
		Quadrant: QuadrantOut{
			Code: code, TechnicalName: s.cfg.TechnicalName(code),
			DisplayName: display, Provisional: s.cfg.PersonProvisional(),
		},
		Complementary: ComplementaryOut{
			Codes: comp.Codes, TemperatureRule: comp.Temperature,
			Provisional: s.cfg.ComplementaryProvisional(),
		},
		Labels: LabelsOut{
			Undertone:          s.cfg.Presentation.Undertone[temp],
			Value:              s.cfg.Presentation.ValueLabel[gv],
			Chroma:             s.cfg.Presentation.ChromaLabel[gc],
			SeasonEquivalents:  s.cfg.Presentation.SeasonEquivalents[code][temp],
			SeasonsProvisional: s.cfg.Presentation.Status != domain.StatusWCPA,
		},
		FoundationBand:  s.cfg.FoundationBand(skin[0]),
		Flags:           flags,
		QualityFailed:   m.Quality.QCFailed,
		Recommendations: recs,
		Catalog: s.catalog(shades, func(sh domain.ShadeCandidate) string {
			_, status, _ := s.cfg.Rate(code, sh)
			return status
		}),
		ConfigVersion: s.cfg.Version,
	}
	// Raw colorimetry never reaches the user unless both the deployment and
	// the request ask for it.
	if debug && s.debugEnabled {
		out.Debug = map[string]interface{}{
			"measurement": m.Measurement, "munsell": m.Munsell,
			"valueGroup": gv, "chromaGroup": gc, "distanceToCentre": dist,
			"bMinusA": math.Round(bMinusA*100) / 100, "pipelineVersion": m.PipelineVersion,
		}
	}
	return out, nil
}

// Catalog lists every shade to try, without an analysis (no status).
func (s *Service) Catalog(ctx context.Context) (*CatalogResponse, error) {
	shades, err := s.store.ListCandidates(ctx)
	if err != nil {
		return nil, err
	}
	return &CatalogResponse{Catalog: s.catalog(shades, nil), ConfigVersion: s.cfg.Version}, nil
}

// catalog groups shades by category: products by name, and each product's
// shades light to dark (L*) so "Warna Tersedia" reads as a range. rate gives
// a shade's status for a rated category; nil leaves every status empty.
func (s *Service) catalog(shades []Candidate, rate func(domain.ShadeCandidate) string) map[string][]CatalogShadeOut {
	sorted := append([]Candidate(nil), shades...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		switch {
		case a.ProductName != b.ProductName:
			return a.ProductName < b.ProductName
		case a.ProductID != b.ProductID:
			return a.ProductID < b.ProductID
		case a.Lab[0] != b.Lab[0]:
			return a.Lab[0] > b.Lab[0]
		}
		return a.ShadeName < b.ShadeName
	})
	out := map[string][]CatalogShadeOut{}
	for _, c := range sorted {
		status := ""
		if rate != nil && s.cfg.Rates(c.Category) {
			status = rate(c.ShadeCandidate)
		}
		out[c.Category] = append(out[c.Category], CatalogShadeOut{
			ShadeID: c.ShadeID, ProductID: c.ProductID, ProductName: c.ProductName,
			ShadeName: c.ShadeName, HexColor: c.HexColor, HueName: s.cfg.HueNameOf(c.ShadeCandidate),
			Status: status, ColourSource: c.ColourSource, Mode: c.Mode,
		})
	}
	return out
}

func lRange(sites map[string]float64) float64 {
	if len(sites) == 0 {
		return 0
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, l := range sites {
		lo, hi = math.Min(lo, l), math.Max(hi, l)
	}
	return hi - lo
}

// TryOn renders the selected shades (at most one per category) on the photo.
// The frontend only sends shade ids; colour and render parameters come from
// the catalog, never from the client.
func (s *Service) TryOn(ctx context.Context, image []byte, filename string, shadeIDs []string) ([]byte, error) {
	if len(image) == 0 {
		return nil, fmt.Errorf("%w: image is required", domain.ErrInvalidInput)
	}
	if len(shadeIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one shadeId is required", domain.ErrInvalidInput)
	}
	found, err := s.store.GetCandidates(ctx, shadeIDs)
	if err != nil {
		return nil, err
	}
	byID := map[string]Candidate{}
	for _, c := range found {
		byID[c.ShadeID] = c
	}
	seen := map[string]string{}
	var layers []workerclient.Layer
	for _, id := range shadeIDs {
		c, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s", domain.ErrShadeNotFound, id)
		}
		if prev, dup := seen[c.Category]; dup {
			return nil, fmt.Errorf("%w: shades %s and %s are both %s", domain.ErrInvalidInput, prev, id, c.Category)
		}
		seen[c.Category] = id
		layers = append(layers, workerclient.Layer{
			Category: c.Category, HexColor: c.HexColor,
			Alpha: c.Alpha, Specular: c.Specular, Mode: c.Mode,
		})
	}
	return s.worker.Render(ctx, image, filename, layers)
}

// Quadrants lists the quadrant legend for the UI.
func (s *Service) Quadrants() []QuadrantOut {
	out := make([]QuadrantOut, 0, len(s.cfg.Quadrants))
	for _, q := range s.cfg.Quadrants {
		var display *string
		if n := s.cfg.Presentation.DisplayNames[q.Code]; n != "" {
			display = &n
		}
		out = append(out, QuadrantOut{Code: q.Code, TechnicalName: q.TechnicalName, DisplayName: display, Provisional: s.cfg.PersonProvisional()})
	}
	return out
}
