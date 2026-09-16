package domain

type RecommendedCatalogProduct struct {
	SKU         string   `json:"sku"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	TargetZones []string `json:"targetZones"`
	Reason      string   `json:"reason"`
}

// MatchProducts maps fused dimension scores and missing zones into a curated multi-brand regimen.
func MatchProducts(brandId string, scores map[string]float64, missedZones []string) []RecommendedCatalogProduct {
	var recs []RecommendedCatalogProduct
	uvScore := scores["UV_DEFENSE"]
	longevityScore := scores["SKIN_LONGEVITY"]

	switch brandId {
	case "brand_kahf":
		if uvScore < 85.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "K-SUN-STICK-SPF50",
				Name:        "Kahf Triple+ Protection Sunscreen Stick SPF 50+ PA++++",
				Category:    "Sunscreen",
				TargetZones: missedZones,
				Reason:      "Proteksi maksimal tahan keringat untuk zona yang belum terlindungi.",
			})
		}
		if longevityScore < 80.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "K-AGE-DEFENSE-MOIST",
				Name:        "Kahf Reverent Anti-Pollution & Age Defense Moisturizer",
				Category:    "Moisturizer",
				TargetZones: []string{"ZONE_FOREHEAD", "ZONE_CHIN_JAWLINE"},
				Reason:      "Memperlambat penuaan dini dan degradasi kolagen pria.",
			})
		}
	case "brand_makeover":
		if uvScore < 85.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "MO-URBAN-SHIELD-50",
				Name:        "Make Over Powerskin Urban Defense Sunscreen SPF 50 PA++++",
				Category:    "Sunscreen",
				TargetZones: missedZones,
				Reason:      "Proteksi UV ringan optimal tanpa white cast di bawah riasan.",
			})
		}
		if longevityScore < 80.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "MO-RADIANCE-PRIMER",
				Name:        "Make Over Powerstay Radiance Booster Anti-Aging Serum Primer",
				Category:    "Serum",
				TargetZones: []string{"ZONE_PERIORBITAL_EYELIDS", "ZONE_LEFT_CHEEK_INNER", "ZONE_RIGHT_CHEEK_INNER"},
				Reason:      "Menghaluskan tekstur garis halus dan meningkatkan kilau kulit.",
			})
		}
	default: // brand_wardah
		if uvScore < 85.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "W-UV-SHIELD-SPF50",
				Name:        "Wardah UV Shield Essential Sunscreen Gel SPF 50 PA++++",
				Category:    "Sunscreen",
				TargetZones: missedZones,
				Reason:      "Perlindungan broad spectrum untuk menutup area yang belum terlindungi.",
			})
		}
		if longevityScore < 80.0 {
			recs = append(recs, RecommendedCatalogProduct{
				SKU:         "W-RENEW-PEPTIDE-SERUM",
				Name:        "Wardah Renew You Anti-Aging Intensive Serum with Matrixyl Peptide",
				Category:    "Serum",
				TargetZones: []string{"ZONE_FOREHEAD", "ZONE_PERIORBITAL_EYELIDS", "ZONE_CHIN_JAWLINE"},
				Reason:      "Meningkatkan elastisitas kolagen dan menyamarkan garis halus.",
			})
		}
		recs = append(recs, RecommendedCatalogProduct{
			SKU:         "W-HYDRA-BARRIER-GEL",
			Name:        "Wardah Hydra Rose Moisture Rich Night Gel",
			Category:    "Moisturizer",
			TargetZones: []string{"ZONE_LEFT_CHEEK_INNER", "ZONE_RIGHT_CHEEK_INNER", "ZONE_T_ZONE_NOSE"},
			Reason:      "Mengunci kelembapan dan memperkuat skin barrier.",
		})
	}

	return recs
}
