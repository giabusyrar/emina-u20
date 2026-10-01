package domain

// ValidateRequest checks transport-level invariants only. Dimension and skin
// condition codes are validated against Reference Data by the resolver.
func ValidateRequest(req *AnalyzeVisionRequest) *ValidationErrorResponse {
	// No default tenant: a request without one would be analysed against
	// another brand's configuration.
	var missing []ValidationErrorItem
	if req.BrandID == "" {
		missing = append(missing, ValidationErrorItem{Field: "brandId", Value: "empty", Reason: "brandId is required"})
	}
	if req.ApplicationID == "" {
		missing = append(missing, ValidationErrorItem{Field: "applicationId", Value: "empty", Reason: "applicationId is required"})
	}
	if len(missing) > 0 {
		return &ValidationErrorResponse{
			Error:             "BAD_REQUEST",
			Message:           "Validation failed on requested parameters",
			InvalidParameters: missing,
		}
	}
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
