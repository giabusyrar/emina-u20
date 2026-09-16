package domain

// ValidateRequest checks transport-level invariants only. Dimension and skin
// condition codes are validated against Reference Data by the resolver.
func ValidateRequest(req *AnalyzeVisionRequest) *ValidationErrorResponse {
	if len(req.Images) == 0 {
		return &ValidationErrorResponse{
			Error:   "BAD_REQUEST",
			Message: "Validation failed on requested parameters",
			InvalidParameters: []ValidationErrorItem{{
				Field:  "images",
				Value:  "empty",
				Reason: "at least one facial image is required (image_front, image_left, or image_right)",
			}},
		}
	}
	return nil
}
