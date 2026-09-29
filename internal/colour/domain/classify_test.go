package domain

import (
	"path/filepath"
	"testing"
)

func loadTestConfig(t *testing.T) *Config {
	t.Helper()
	c, err := LoadConfig(filepath.Join("..", "..", "..", "config", "wcpa.json"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return c
}

// Values from the notebook export wcpa_20260918_064150.json ("alis tipis.jpeg").
func TestPersonMatchesNotebookExport(t *testing.T) {
	c := loadTestConfig(t)
	skin := Lab{74.8, 8.55, 11.31}

	code, gv, gc, _ := c.PersonQuadrant(7.41, 2.84)
	if code != "HV_LC" || gv != "high" || gc != "low" {
		t.Fatalf("got %s %s %s, notebook: High Value x Low Chroma, high, low", code, gv, gc)
	}
	temp, d := c.TemperatureOf(skin)
	if temp != "cool" || d < 2.75 || d > 2.77 {
		t.Fatalf("temperature %s (b-a %.2f), notebook: cool (2.76)", temp, d)
	}
	if fb := c.FoundationBand(skin[0]); fb != "very light" {
		t.Fatalf("foundation band %q, notebook: very light", fb)
	}
	comp := c.Complementary.Map[code]
	if len(comp.Codes) != 1 || comp.Codes[0] != "HV_HC" || comp.Temperature != "neutral_to_cool" {
		t.Fatalf("complementary %+v, notebook: [High Value x High Chroma] neutral_to_cool", comp)
	}
	if !c.PersonProvisional() || !c.ComplementaryProvisional() {
		t.Fatal("person thresholds and complementary map must report provisional")
	}
}

func TestTemperatureThresholds(t *testing.T) {
	c := loadTestConfig(t)
	cases := []struct {
		lab  Lab
		want string
	}{
		{Lab{60, 10, 15.6}, "warm"},    // b-a = 5.6
		{Lab{60, 10, 15.0}, "neutral"}, // 5.0
		{Lab{60, 10, 14.4}, "cool"},    // 4.4
	}
	for _, tc := range cases {
		if got, _ := c.TemperatureOf(tc.lab); got != tc.want {
			t.Errorf("%v: got %s want %s", tc.lab, got, tc.want)
		}
	}
}

func TestShadeBandsFromSlide(t *testing.T) {
	c := loadTestConfig(t)
	cases := []struct {
		v, ch        float64
		wantCode     string
		wantV, wantC string
	}{
		{8.5, 7.5, "HV_HC", "high", "high"},
		{8.5, 4.0, "HV_LC", "high", "low"},
		{2.0, 7.0, "LV_HC", "low", "high"},
		{3.5, 4.0, "LV_LC", "low", "low"},   // 3.5 is inside the new deep band
		{5.25, 5.75, "MM", "medium", "low"}, // MM centre
	}
	for _, tc := range cases {
		code, gv, gc := c.ShadeQuadrant(tc.v, tc.ch)
		if code != tc.wantCode || gv != tc.wantV || gc != tc.wantC {
			t.Errorf("V%.2f C%.2f: got %s %s %s want %s %s %s", tc.v, tc.ch, code, gv, gc, tc.wantCode, tc.wantV, tc.wantC)
		}
	}
}

func TestRecommendOrdersSameQuadrantFirst(t *testing.T) {
	c := loadTestConfig(t)
	shades := []ShadeCandidate{
		{ShadeID: "comp", Lab: Lab{70, 20, 22}, V: 8.5, C: 7.5},  // HV_HC, b-a 2 -> cool: allowed
		{ShadeID: "same", Lab: Lab{70, 10, 12}, V: 8.4, C: 4.3},  // HV_LC
		{ShadeID: "warmc", Lab: Lab{70, 10, 30}, V: 8.5, C: 7.5}, // HV_HC but warm: dropped
		{ShadeID: "out", Lab: Lab{30, 10, 12}, V: 2.0, C: 4.0},   // LV_LC: dropped
	}
	got := c.Recommend("HV_LC", 7.41, 2.84, shades)
	if len(got) != 2 || got[0].ShadeID != "same" || got[1].ShadeID != "comp" {
		ids := []string{}
		for _, r := range got {
			ids = append(ids, r.ShadeID+":"+r.Status)
		}
		t.Fatalf("got %v, want [same:same_quadrant comp:complementary]", ids)
	}
}

func TestRateMatchesRecommend(t *testing.T) {
	c := loadTestConfig(t)
	shades := []ShadeCandidate{
		{ShadeID: "comp", Lab: Lab{70, 20, 22}, V: 8.5, C: 7.5},
		{ShadeID: "same", Lab: Lab{70, 10, 12}, V: 8.4, C: 4.3},
		{ShadeID: "warmc", Lab: Lab{70, 10, 30}, V: 8.5, C: 7.5},
		{ShadeID: "out", Lab: Lab{30, 10, 12}, V: 2.0, C: 4.0},
	}
	want := map[string]string{"comp": RatingComplementary, "same": RatingSameQuadrant, "warmc": "", "out": ""}
	for _, s := range shades {
		if _, got, _ := c.Rate("HV_LC", s); got != want[s.ShadeID] {
			t.Errorf("%s: rating %q want %q", s.ShadeID, got, want[s.ShadeID])
		}
	}
}

func TestRatedAndNamedCategories(t *testing.T) {
	c := loadTestConfig(t)
	for _, k := range []string{"lip", "blush", "complexion", "eyeshadow"} {
		if !c.Rates(k) {
			t.Errorf("%s should be rated", k)
		}
	}
	for _, k := range []string{"brow", "eyeliner", "mascara"} {
		if c.Rates(k) {
			t.Errorf("%s is black/brown: a skin quadrant says nothing about it", k)
		}
	}
	black := ShadeCandidate{Category: "mascara", Lab: Lab{0, 0, 0}}
	if got := c.HueNameOf(black); got != "" {
		t.Errorf("black mascara hue name %q, want none", got)
	}
	lip := ShadeCandidate{Category: "lip", Lab: Lab{51.68, 28.82, 16.21}}
	if got := c.HueNameOf(lip); got != "Peach" {
		t.Errorf("lip hue name %q, want Peach", got)
	}
	var all Config
	if !all.Rates("mascara") || !all.NamesHue("mascara") {
		t.Error("empty lists must mean every category (notebook behaviour)")
	}
}

func TestSourceRank(t *testing.T) {
	if !(SourceRank(SourceCube) < SourceRank(SourceSwatch) && SourceRank(SourceSwatch) < SourceRank(SourceEstimate) &&
		SourceRank(SourceEstimate) < SourceRank("unknown")) {
		t.Fatal("cube > swatch > estimate > unknown")
	}
}

func TestHueNames(t *testing.T) {
	c := loadTestConfig(t)
	// Notebook export: lip Lab 51.68/28.82/16.21 -> "Peach" (hue 29.4°).
	if got := c.HueName(Lab{51.68, 28.82, 16.21}); got != "Peach" {
		t.Fatalf("got %s want Peach", got)
	}
}
