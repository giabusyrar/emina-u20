package entity

import "testing"

func TestShadeTableName(t *testing.T) {
	if got := (Shade{}).TableName(); got != "match_shades" {
		t.Errorf("Shade.TableName() = %q, want %q", got, "match_shades")
	}
}

func TestShadeAssetTableName(t *testing.T) {
	if got := (ShadeAsset{}).TableName(); got != "match_shade_assets" {
		t.Errorf("ShadeAsset.TableName() = %q, want %q", got, "match_shade_assets")
	}
}
