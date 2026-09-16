package domain_test

import (
	"testing"

	"github.com/experience-gateway/apps/core-engine/pkg/vision/domain"
	"github.com/stretchr/testify/assert"
)

func TestValidateRequest_NoImages(t *testing.T) {
	req := &domain.AnalyzeVisionRequest{
		Images: []domain.RawImageInput{},
	}
	errResp := domain.ValidateRequest(req)
	assert.NotNil(t, errResp)
	assert.Equal(t, "BAD_REQUEST", errResp.Error)
	assert.Len(t, errResp.InvalidParameters, 1)
	assert.Equal(t, "images", errResp.InvalidParameters[0].Field)
}

func TestValidateRequest_LeavesCodeValidationToTheResolver(t *testing.T) {
	req := &domain.AnalyzeVisionRequest{
		Images: []domain.RawImageInput{
			{Angle: domain.AngleFront, FileName: "front.jpg", Data: []byte("dummy")},
		},
		Dimensions:     []string{"wrinkle"},
		SkinConditions: []string{"concern_acne_papule"},
	}
	assert.Nil(t, domain.ValidateRequest(req))
}
