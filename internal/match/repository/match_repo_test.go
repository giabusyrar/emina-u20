package repository

import (
	"context"
	"testing"

	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
)

func TestShadeCRUD_InMemory(t *testing.T) {
	repo := NewMatchRepo(nil) // nil DB => in-memory fallback, same pattern as products/conflicts

	created, err := repo.CreateShade(context.Background(), entity.Shade{
		ProductID: "prod-1",
		Name:      "Ruby Red",
		HexColor:  "#C41E3A",
		Region:    "lip",
	})
	if err != nil {
		t.Fatalf("CreateShade error: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateShade did not assign an ID")
	}
	if created.ExtractionStatus != "pending" {
		t.Errorf("ExtractionStatus = %q, want %q", created.ExtractionStatus, "pending")
	}

	list, err := repo.ListShades(context.Background(), "prod-1")
	if err != nil {
		t.Fatalf("ListShades error: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("ListShades = %+v, want one shade with ID %q", list, created.ID)
	}

	created.ExtractionStatus = "ready"
	updated, err := repo.UpdateShade(context.Background(), *created)
	if err != nil {
		t.Fatalf("UpdateShade error: %v", err)
	}
	if updated.ExtractionStatus != "ready" {
		t.Errorf("after update, ExtractionStatus = %q, want %q", updated.ExtractionStatus, "ready")
	}

	if err := repo.DeleteShade(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteShade error: %v", err)
	}
	list, _ = repo.ListShades(context.Background(), "prod-1")
	if len(list) != 0 {
		t.Errorf("after delete, ListShades = %+v, want empty", list)
	}
}

func TestShadeAsset_InMemory(t *testing.T) {
	repo := NewMatchRepo(nil)

	asset, err := repo.SaveShadeAsset(context.Background(), entity.ShadeAsset{
		ShadeID:     "shade-1",
		ColorMapURL: "s3://bucket/shade-1/color.png",
		AlphaMapURL: "s3://bucket/shade-1/alpha.png",
	})
	if err != nil {
		t.Fatalf("SaveShadeAsset error: %v", err)
	}
	if asset.ID == "" {
		t.Fatal("SaveShadeAsset did not assign an ID")
	}

	got, err := repo.GetShadeAsset(context.Background(), "shade-1")
	if err != nil {
		t.Fatalf("GetShadeAsset error: %v", err)
	}
	if got.ColorMapURL != asset.ColorMapURL {
		t.Errorf("GetShadeAsset ColorMapURL = %q, want %q", got.ColorMapURL, asset.ColorMapURL)
	}

	byID, err := repo.GetShadeAssetByID(context.Background(), asset.ID)
	if err != nil {
		t.Fatalf("GetShadeAssetByID error: %v", err)
	}
	if byID.ShadeID != "shade-1" {
		t.Errorf("GetShadeAssetByID ShadeID = %q, want %q", byID.ShadeID, "shade-1")
	}
}
