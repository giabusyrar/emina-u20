package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceOf(t *testing.T) {
	for tag, want := range map[string]string{"[SWATCH]": "swatch", "swatch": "swatch", " [PERKIRAAN] ": "estimate", "[perkiraan]": "estimate"} {
		if got, err := sourceOf(tag); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", tag, got, err, want)
		}
	}
	for _, tag := range []string{"", "[KARANGAN]", "cube"} {
		if _, err := sourceOf(tag); err == nil {
			t.Errorf("%q must be refused: a colour of unknown origin cannot look real", tag)
		}
	}
}

func TestBrandMap(t *testing.T) {
	m, err := parseBrandMap("Make Over=b-mo, wardah = b-wd,OMG=b-omg,")
	if err != nil || m["make over"] != "b-mo" || m["wardah"] != "b-wd" || m["omg"] != "b-omg" {
		t.Fatalf("got %v %v", m, err)
	}
	if _, err := parseBrandMap("Wardah"); err == nil {
		t.Fatal("entry without an id must be refused")
	}
	rows := []csvRow{{line: 2, brand: "Make Over"}, {line: 3, brand: "WARDAH"}, {line: 4}}
	ids, err := brandIDs(rows, "b-default", m)
	if err != nil || ids[0] != "b-mo" || ids[1] != "b-wd" || ids[2] != "b-default" {
		t.Fatalf("got %v %v", ids, err)
	}
	if _, err := brandIDs([]csvRow{{line: 2, brand: "Emina"}}, "b-default", m); err == nil || !strings.Contains(err.Error(), "Emina") {
		t.Fatalf("an unmapped brand must be refused, not put under -brand: %v", err)
	}
	if _, err := brandIDs([]csvRow{{line: 7}}, "", m); err == nil || !strings.Contains(err.Error(), "line 7") {
		t.Fatalf("a row without brand and no -brand must be refused: %v", err)
	}
}

// The demo catalog in seed/ must parse, and every section must have
// products to try (brow and eyeliner only black/brown; mascara by type).
func TestDemoCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "seed", "vto_products_demo.csv")
	if _, err := os.Stat(path); err != nil {
		t.Skip("seed/vto_products_demo.csv not present")
	}
	rows, err := readCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	products := map[string]map[string]bool{}
	mascaraTypes := map[string]bool{}
	for _, r := range rows {
		if _, ok := regionFor[r.kategori]; !ok {
			t.Fatalf("line %d: unknown kategori %q", r.line, r.kategori)
		}
		if r.brand == "" {
			t.Fatalf("line %d: demo rows must name their brand", r.line)
		}
		if products[r.kategori] == nil {
			products[r.kategori] = map[string]bool{}
		}
		products[r.kategori][r.produk] = true
		if r.kategori == "mascara" {
			mascaraTypes[r.mode] = true
		}
	}
	for _, k := range []string{"brow", "lip", "complexion", "blush", "eyeliner", "eyeshadow", "mascara"} {
		if len(products[k]) < 2 {
			t.Errorf("%s: %d products, want at least 2", k, len(products[k]))
		}
	}
	for _, typ := range []string{"natural", "volumizing", "lengthening"} {
		if !mascaraTypes[typ] {
			t.Errorf("no %s mascara", typ)
		}
	}
	if _, err := brandIDs(rows, "", map[string]string{"make over": "a", "wardah": "b", "omg": "c"}); err != nil {
		t.Fatal(err)
	}
}
