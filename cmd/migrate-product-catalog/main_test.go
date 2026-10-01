package main

import "testing"

// The id is kept, so match_shades.product_id and product groups still point
// at it; key actives become the description, which match reads back as key
// actives.
func TestToProductKeepsIDAndKeyActives(t *testing.T) {
	old := oldProduct{ID: "clr-prd-e41ea982f8fcb07e", BrandID: "brd-1", Name: "Make Over Blush On Single",
		KeyActives: "pigment", ImageURL: "https://x/y.png", IsActive: true}
	p := toProduct(old, "cat-blush", "")
	if p.ID != old.ID || p.BrandID != "brd-1" || p.Name != old.Name || p.CategoryID != "cat-blush" ||
		p.TextureID != "" || p.Description != "pigment" || p.ImageURL != old.ImageURL || !p.IsActive {
		t.Errorf("got %+v", p)
	}
}
