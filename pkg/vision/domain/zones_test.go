package domain_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/stretchr/testify/assert"
)

func TestStandardFacialZones(t *testing.T) {
	zones := domain.GetStandardFacialZones()
	assert.Len(t, zones, 8)

	zoneCodes := make(map[string]bool)
	for _, z := range zones {
		zoneCodes[z.Code] = true
		assert.NotEmpty(t, z.Name)
		assert.NotEmpty(t, z.DefaultPolygon)
	}

	assert.True(t, zoneCodes[domain.ZoneForehead])
	assert.True(t, zoneCodes[domain.ZoneTZoneNose])
	assert.True(t, zoneCodes[domain.ZoneLeftCheekInner])
	assert.True(t, zoneCodes[domain.ZoneLeftCheekOuter])
	assert.True(t, zoneCodes[domain.ZoneRightCheekInner])
	assert.True(t, zoneCodes[domain.ZoneRightCheekOuter])
	assert.True(t, zoneCodes[domain.ZonePeriorbitalEyelids])
	assert.True(t, zoneCodes[domain.ZoneChinJawline])
}
