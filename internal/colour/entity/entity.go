// Package entity holds the colour domain's own table. Shades and products
// stay in match_* (owned by core-engine's match domain); this table only adds
// the WCPA colorimetry of a shade, per measurement source.
package entity

import "time"

// ShadeColorimetry is a shade's colour in WCPA terms from one source.
// The hex gives a swatch row (the product's swatch, [SWATCH] in the catalog
// CSV) or an estimate row (estimated for a demo, [PERKIRAAN]); once the
// catalog is measured on skin a cube row is added. The best source wins:
// cube, then swatch, then estimate (domain.SourceRank).
type ShadeColorimetry struct {
	ID       string `json:"id" gorm:"type:varchar(64);primaryKey"`
	ShadeID  string `json:"shadeId" gorm:"type:varchar(64);not null;uniqueIndex:idx_colorimetry_shade_source"`
	Source   string `json:"source" gorm:"type:varchar(32);not null;uniqueIndex:idx_colorimetry_shade_source"` // cube | swatch | estimate
	Category string `json:"category" gorm:"type:varchar(32);not null;index"`                                  // lip | blush | brow | eyeliner | mascara | eyeshadow | complexion

	LabL float64 `json:"labL"`
	LabA float64 `json:"labA"`
	LabB float64 `json:"labB"`

	MunsellV     float64 `json:"munsellV"`
	MunsellC     float64 `json:"munsellC"`
	ChromaSource string  `json:"chromaSource" gorm:"type:varchar(32)"` // colour-science | approximation

	// Render parameters from the catalog (vto_products.csv). [KARANGAN] in the
	// notebook until measured on skin.
	Alpha    float64 `json:"alpha"`
	Specular float64 `json:"specular"`
	Mode     string  `json:"mode" gorm:"type:varchar(32)"`

	CreatedAt time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty" gorm:"autoUpdateTime"`
}

func (ShadeColorimetry) TableName() string {
	return "match_shade_colorimetry"
}
