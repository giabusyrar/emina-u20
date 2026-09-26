package domain

import (
	"testing"
)

func TestMatchProducts_MultiBrand(t *testing.T) {
	scores := map[string]float64{
		"UV_DEFENSE":     60.0,
		"SKIN_LONGEVITY": 70.0,
	}
	missedZones := []string{"ZONE_FOREHEAD", "ZONE_PERIORBITAL_EYELIDS"}

	wardahRecs := MatchProducts("brand_wardah", scores, missedZones)
	if len(wardahRecs) == 0 || wardahRecs[0].SKU != "W-UV-SHIELD-SPF50" {
		t.Errorf("expected Wardah sunscreen recommendation, got %+v", wardahRecs)
	}

	kahfRecs := MatchProducts("brand_kahf", scores, missedZones)
	if len(kahfRecs) == 0 || kahfRecs[0].SKU != "K-SUN-STICK-SPF50" {
		t.Errorf("expected Kahf sunscreen recommendation, got %+v", kahfRecs)
	}

	makeoverRecs := MatchProducts("brand_makeover", scores, missedZones)
	if len(makeoverRecs) == 0 || makeoverRecs[0].SKU != "MO-URBAN-SHIELD-50" {
		t.Errorf("expected Make Over sunscreen recommendation, got %+v", makeoverRecs)
	}
}
