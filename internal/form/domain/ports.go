package domain

import "io"

// Ruleset is the subset of a score ruleset the Form Engine reads: enough to
// find a survey's ruleset and read field_mapping/dimension_fusion out of its
// Schema, without depending on score's entity package.
type Ruleset struct {
	ID     string
	Code   string
	Status string
	Schema string
}

// VisionSubmission is what the caller already has in hand before it can ask
// the vision vendor (Paradev) anything: the applicant's personal data
// (consent already collected) and their photo(s).
type VisionSubmission struct {
	FullName              string
	PhoneNumber           string
	Email                 string
	DateOfBirth           string // YYYY-MM-DD
	ConsentDataProcessing bool
	ConsentMarketing      bool
	Image                 io.Reader
	ImageFilename         string
}

// VisionResult is the vendor's response, reduced to what the rest of the
// orchestration needs: every numeric leaf value the vendor returned,
// flattened by its full dot-path from the response root (matching how
// field_mapping's vision codes are recorded — e.g.
// "inference_result.results.skin_scoring.Pores"), plus a trace id for support.
type VisionResult struct {
	Flattened     map[string]float64
	TransactionID string
	Raw           map[string]interface{}
}
