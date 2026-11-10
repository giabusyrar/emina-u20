package domain

import (
	"math"
	"strings"
	"testing"
)

// Test pairs from Sharma, Wu & Dalal 2005, table 1 (also reproduced by
// colour-science's delta_E CIE 2000).
func TestDeltaE2000Sharma(t *testing.T) {
	cases := []struct {
		a, b Lab
		want float64
	}{
		{Lab{50, 2.6772, -79.7751}, Lab{50, 0, -82.7485}, 2.0425},
		{Lab{50, 0, 0}, Lab{50, -1, 2}, 2.3669},
		{Lab{50, 2.5, 0}, Lab{73, 25, -18}, 27.1492},
		{Lab{60.2574, -34.0099, 36.2677}, Lab{60.4626, -34.1751, 39.4387}, 1.2644},
		{Lab{22.7233, 20.0904, -46.6940}, Lab{23.0331, 14.9730, -42.5619}, 2.0373},
		{Lab{90.8027, -2.0831, 1.4410}, Lab{91.1528, -1.6435, 0.0447}, 1.4441},
	}
	for _, c := range cases {
		if got := DeltaE2000(c.a, c.b); math.Abs(got-c.want) > 1e-4 {
			t.Errorf("ΔE2000(%v, %v) = %.4f, want %.4f", c.a, c.b, got, c.want)
		}
		if got := DeltaE2000(c.b, c.a); math.Abs(got-c.want) > 1e-4 {
			t.Errorf("ΔE2000 must be symmetric: %.4f", got)
		}
	}
	if DeltaE2000(Lab{40, 10, 10}, Lab{40, 10, 10}) != 0 {
		t.Error("identical colours differ")
	}
}

func TestISCCNBSCentroids(t *testing.T) {
	cs, err := ISCCNBSCentroids()
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].Number != 1 || cs[0].Name != "vivid pink" || cs[266].Name != "black" {
		t.Errorf("table order broken: %+v … %+v", cs[0], cs[266])
	}
	open := 0
	for i, c := range cs {
		if c.Number != i+1 || c.Name == "" || c.Lab[0] < 0 || c.Lab[0] > 100 {
			t.Errorf("bad centroid %+v", c)
		}
		if c.Open {
			open++
		}
	}
	if open != 32 {
		t.Errorf("open-chroma blocks = %d, want 32 (Kelly's '+')", open)
	}
}

// The names the brief's hue-angle families got wrong: saturated reds read
// "Peach", a light pink read "Red". Expected names from the same nearest-
// centroid rule computed with colour-science (sRGB D65 -> CIELAB, CIE 2000).
func TestISCCNBSNames(t *testing.T) {
	cases := []struct {
		hex  string
		lab  Lab
		want string
	}{
		{"#D22E30 Wardah 06 Feeling Red", Lab{46.80, 62.35, 39.76}, "vivid red"},
		{"#D11D22 Make Over C05 Swift", Lab{45.01, 65.80, 45.49}, "vivid red"},
		{"#A92431 Wardah 01 Red-dicted", Lab{37.70, 53.26, 26.68}, "vivid red"},
		{"#F18070 Summer Peach", Lab{65.99, 41.76, 28.30}, "deep yellowish pink"},
		{"#DE929F 10 Berry Pretty", Lab{68.45, 30.41, 5.07}, "moderate purplish pink"},
		{"#F9D7C1 N10 Marble", Lab{88.25, 8.52, 15.23}, "pale yellowish pink"},
	}
	for _, c := range cases {
		got, err := ISCCNBSName(c.lab)
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.hex, got, err, c.want)
		}
	}
}

func TestHueNamesUseISCCNBS(t *testing.T) {
	c := loadTestConfig(t)
	// The notebook's example lip (Lab 51.68/28.82/16.21) was "Peach" by hue
	// angle; it is a muted red.
	if got := c.HueName(Lab{51.68, 28.82, 16.21}); got != "grayish red" {
		t.Fatalf("got %q want grayish red", got)
	}
	if strings.Contains(c.HueName(Lab{46.80, 62.35, 39.76}), "pink") {
		t.Error("a vivid red must not be named a pink")
	}
}
