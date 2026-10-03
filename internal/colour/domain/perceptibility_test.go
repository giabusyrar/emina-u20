package domain

import (
	"path/filepath"
	"testing"
)

// The bands are facts of human vision recorded in AGENTS.md; pin them so a
// change has to be deliberate.
func TestPerceptibilityThresholdsAreTheDocumentedOnes(t *testing.T) {
	if DeltaE00JND != 1.0 || DeltaE00CloseInspection != 2.0 || DeltaE00AtAGlance != 5.0 {
		t.Fatalf("bands changed: %v %v %v", DeltaE00JND, DeltaE00CloseInspection, DeltaE00AtAGlance)
	}
}

func TestPerceptibilityBandBoundaries(t *testing.T) {
	for _, tc := range []struct {
		dE   float64
		want string
	}{
		{0, BandNotPerceptible}, {0.99, BandNotPerceptible},
		{1.0, BandCloseInspection}, {1.99, BandCloseInspection},
		{2.0, BandNoticeable}, {4.99, BandNoticeable},
		{5.0, BandAtAGlance}, {12, BandAtAGlance},
	} {
		if got := PerceptibilityBand(tc.dE); got != tc.want {
			t.Errorf("PerceptibilityBand(%v) = %s, want %s", tc.dE, got, tc.want)
		}
	}
}

func TestSkinMatchConfig(t *testing.T) {
	c, err := LoadConfig(filepath.Join("..", "..", "..", "config", "wcpa.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.MatchesSkin("complexion") || c.MatchesSkin("lip") {
		t.Fatalf("skinMatch.categories = %v", c.SkinMatch.Categories)
	}
	c.SkinMatch.Categories = nil
	if c.MatchesSkin("complexion") {
		t.Fatal("an empty skinMatch list means none, not every category")
	}
}

func TestSkinMatchToleranceMustBePositive(t *testing.T) {
	c, err := LoadConfig(filepath.Join("..", "..", "..", "config", "wcpa.json"))
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	c.SkinMatch.MaxDeltaE00 = &zero
	if err := c.validate(); err == nil {
		t.Fatal("a zero tolerance must be refused")
	}
}
