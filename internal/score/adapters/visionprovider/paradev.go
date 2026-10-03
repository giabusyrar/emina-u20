// Package visionprovider adapts vision vendors to the Score Engine's
// domain.VisionProvider interface. Paradev is the first implementation —
// it wraps the existing form-engine visionclient.Client rather than
// re-implementing the vendor's multipart request shape (static fields,
// manual image Content-Type) a second time.
package visionprovider

import (
	"context"
	"fmt"

	scoredomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/adapters/visionclient"
)

// ParadevCode is this provider's vision_source_code — matched against a
// ruleset schema's own vision_source_code field to resolve which
// VisionProvider an evaluate call should use.
const ParadevCode = "paradev_skin_analyzer"

type Paradev struct {
	client *visionclient.Client
}

// NewParadev wraps an existing form-engine visionclient.Client — the same
// instance already constructed for the (still-live, pre-Fase-5) form-engine
// orchestration, so there is only ever one HTTP client/connection pool
// talking to Paradev, not two independently configured copies.
func NewParadev(client *visionclient.Client) *Paradev {
	return &Paradev{client: client}
}

func (p *Paradev) Code() string { return ParadevCode }

func (p *Paradev) Submit(ctx context.Context, sub scoredomain.VisionSubmission) (*scoredomain.VisionResult, error) {
	result, err := p.client.Submit(ctx, visionclient.Submission{
		FullName:              sub.FullName,
		PhoneNumber:           sub.PhoneNumber,
		Email:                 sub.Email,
		DateOfBirth:           sub.DateOfBirth,
		ConsentDataProcessing: sub.ConsentDataProcessing,
		ConsentMarketing:      sub.ConsentMarketing,
		Image:                 sub.Image,
		ImageFilename:         sub.ImageFilename,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", scoredomain.ErrVisionUnavailable, err)
	}
	return &scoredomain.VisionResult{
		Flattened:     result.Flattened,
		TransactionID: result.TransactionID,
	}, nil
}
