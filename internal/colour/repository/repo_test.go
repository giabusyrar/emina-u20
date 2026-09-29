package repository

import "testing"

func TestPreferBestSource(t *testing.T) {
	rows := []row{
		{ShadeID: "a", Source: "estimate", HexColor: "#111111"},
		{ShadeID: "a", Source: "swatch", HexColor: "#222222"},
		{ShadeID: "b", Source: "cube"},
		{ShadeID: "b", Source: "swatch"},
		{ShadeID: "c", Source: "estimate"},
	}
	got := preferBest(rows)
	if len(got) != 3 {
		t.Fatalf("one candidate per shade, got %d", len(got))
	}
	want := map[string]string{"a": "swatch", "b": "cube", "c": "estimate"}
	for _, c := range got {
		if c.ColourSource != want[c.ShadeID] {
			t.Errorf("%s: source %s want %s", c.ShadeID, c.ColourSource, want[c.ShadeID])
		}
	}
	if got[0].ShadeID != "a" || got[1].ShadeID != "b" || got[2].ShadeID != "c" {
		t.Errorf("order must follow the query: %v %v %v", got[0].ShadeID, got[1].ShadeID, got[2].ShadeID)
	}
}
