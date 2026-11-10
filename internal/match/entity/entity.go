package entity

import "time"

// ProductCatalogItem is match's read model of a product. It is scanned from
// ref_products (reference-service owns that table); it has no table of its own.
type ProductCatalogItem struct {
	ID          string     `json:"id" gorm:"type:varchar(64);primaryKey"`
	BrandID     string     `json:"brandId" gorm:"type:varchar(64);not null;index"`
	Name        string     `json:"name" gorm:"type:varchar(255);not null"`
	Category    string     `json:"category" gorm:"type:varchar(64);not null"` // Cleanser, Serum, Moisturizer, Sunscreen, Toner
	Texture     string     `json:"texture" gorm:"type:varchar(64);not null"`  // Water Gel, Cream, Fluid, Emulsion, Foam
	Ingredients string     `json:"ingredients" gorm:"type:text;not null"`     // Comma-separated list of ingredients
	KeyActives  string     `json:"keyActives" gorm:"type:text"`               // Comma-separated primary actives
	ImageURL    string     `json:"imageUrl" gorm:"type:varchar(512)"`
	IsActive    bool       `json:"isActive" gorm:"default:true"`
	CreatedAt   time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime"`
}

// ConflictMatrixRule prevents active ingredient collisions
type ConflictMatrixRule struct {
	ID               string     `json:"id" gorm:"type:varchar(64);primaryKey"`
	BrandID          string     `json:"brandId" gorm:"type:varchar(64);default:'*';index"`
	ApplicationID    string     `json:"applicationId" gorm:"type:varchar(64);default:'*';index"`
	IngredientA      string     `json:"ingredientA" gorm:"type:varchar(255);not null"`
	IngredientB      string     `json:"ingredientB" gorm:"type:varchar(255);not null"`
	ConflictType     string     `json:"conflictType" gorm:"type:varchar(64);default:'incompatible'"`     // incompatible, pH_clash, over_exfoliation
	ResolutionAction string     `json:"resolutionAction" gorm:"type:varchar(128);default:'split_am_pm'"` // split_am_pm, alternate_days, strict_block
	Severity         string     `json:"severity" gorm:"type:varchar(32);default:'high'"`
	WarningMessage   string     `json:"warningMessage" gorm:"type:text"`
	CreatedAt        time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt        *time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime"`
}

func (ConflictMatrixRule) TableName() string {
	return "match_conflict_matrix_rules"
}

// ProductGroup is a brand-curated list of products (and/or product
// categories) scoped to a marketing campaign. When a brand has one or more
// active groups, the matching engine restricts its candidate product pool
// to only the group's members for that brand.
type ProductGroup struct {
	ID            string     `json:"id" gorm:"type:varchar(64);primaryKey"`
	BrandID       string     `json:"brandId" gorm:"type:varchar(64);not null;index"`
	ApplicationID string     `json:"applicationId" gorm:"type:varchar(64);default:'*';index"`
	Name          string     `json:"name" gorm:"type:varchar(255);not null"`
	Code          string     `json:"code" gorm:"type:varchar(64)"`
	Description   string     `json:"description" gorm:"type:text"`
	ProductIDs    []string   `json:"productIds" gorm:"type:jsonb;serializer:json"`
	Categories    []string   `json:"categories" gorm:"type:jsonb;serializer:json"`
	IsActive      bool       `json:"isActive" gorm:"default:true"`
	CreatedAt     time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime"`
}

func (ProductGroup) TableName() string {
	return "match_product_groups"
}

// Shade is a specific color variant of a product (e.g. "Ruby Red" for a
// lipstick SKU). Extraction produces a photorealistic, reusable makeup
// texture for this exact color, generated once offline and applied live
// many times without regenerating it.
type Shade struct {
	ID                string     `json:"id" gorm:"type:varchar(64);primaryKey"`
	ProductID         string     `json:"productId" gorm:"type:varchar(64);not null;index"`
	Name              string     `json:"name" gorm:"type:varchar(255);not null"`
	HexColor          string     `json:"hexColor" gorm:"type:varchar(7);not null"`      // e.g. "#C41E3A"
	Region            string     `json:"region" gorm:"type:varchar(32);not null;index"` // lip | eye | cheek | skin
	ReferencePhotoURL string     `json:"referencePhotoUrl" gorm:"type:varchar(512)"`
	ExtractionStatus  string     `json:"extractionStatus" gorm:"type:varchar(32);not null;default:'pending';index"` // pending | processing | ready | failed
	FailureReason     string     `json:"failureReason,omitempty" gorm:"type:text"`
	AssetID           *string    `json:"assetId,omitempty" gorm:"type:varchar(64)"`
	CreatedAt         time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt         *time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime"`
}

func (Shade) TableName() string {
	return "match_shades"
}

// ShadeAsset holds the object-storage keys of a Shade's extracted,
// photorealistic UV-space makeup texture maps, produced once by
// tryon-engine and reused by every live try-on session.
type ShadeAsset struct {
	ID      string `json:"id" gorm:"type:varchar(64);primaryKey"`
	ShadeID string `json:"shadeId" gorm:"type:varchar(64);not null;index"`
	// Object keys in the tryon texture bucket (e.g. shades/<id>/color.png),
	// never URLs: GET /shade-assets/:id presigns a fresh link per request.
	// Nullable so AutoMigrate can add them to a populated table; the legacy
	// *_url columns are converted and dropped by cmd/shade-asset-keys.
	ColorMapKey  string    `json:"-" gorm:"type:varchar(512)"`
	AlphaMapKey  string    `json:"-" gorm:"type:varchar(512)"`
	FinishMapKey string    `json:"-" gorm:"type:varchar(512)"`
	CreatedAt    time.Time `json:"createdAt" gorm:"autoCreateTime"`
}

func (ShadeAsset) TableName() string {
	return "match_shade_assets"
}
