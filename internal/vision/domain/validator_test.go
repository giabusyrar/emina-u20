package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

func TestValidateRequest_NoImages(t *testing.T) {
	req := &domain.AnalyzeVisionRequest{
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		Images:        []domain.RawImageInput{},
	}
	errResp := domain.ValidateRequest(req)
	assert.NotNil(t, errResp)
	assert.Equal(t, "BAD_REQUEST", errResp.Error)
	assert.Len(t, errResp.InvalidParameters, 1)
	assert.Equal(t, "images", errResp.InvalidParameters[0].Field)
}

func TestValidateRequest_LeavesCodeValidationToTheResolver(t *testing.T) {
	req := &domain.AnalyzeVisionRequest{
		BrandID:       "wardah",
		ApplicationID: "skinverse",
		Images: []domain.RawImageInput{
			{Angle: domain.AngleFront, FileName: "front.jpg", Data: []byte("dummy")},
		},
		Dimensions:     []string{"wrinkle"},
		SkinConditions: []string{"concern_acne_papule"},
	}
	assert.Nil(t, domain.ValidateRequest(req))
}

func TestValidateRequest_TenantIsRequired(t *testing.T) {
	req := &domain.AnalyzeVisionRequest{
		Images: []domain.RawImageInput{{Angle: domain.AngleFront, FileName: "front.jpg", Data: []byte("dummy")}},
	}
	errResp := domain.ValidateRequest(req)
	if assert.NotNil(t, errResp) {
		fields := []string{errResp.InvalidParameters[0].Field, errResp.InvalidParameters[1].Field}
		assert.Equal(t, []string{"brandId", "applicationId"}, fields)
	}
}
