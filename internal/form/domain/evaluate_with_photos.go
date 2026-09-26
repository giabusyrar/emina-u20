package domain

// EvaluateWithPhotosRequest is the multipart body for
// POST /survey/:code/evaluate-with-photos — the one call a brand's client
// makes on Submit: personal data + consent, the Q1-Q6 answers, and the
// applicant's photo. Everything vision-side (calling Paradev, inverting and
// mapping its result into vision_signals) happens inside this one request.
type EvaluateWithPhotosRequest struct {
	BrandID               string
	ApplicationID         string
	CustomerID            string
	FullName              string
	PhoneNumber           string
	Email                 string
	DateOfBirth           string // YYYY-MM-DD
	ConsentDataProcessing bool
	ConsentMarketing      bool
	// Data/CustomerConditions mirror SurveyEvaluateRequest — the same raw
	// Q1-Q6 answers a brand would send to the plain /evaluate endpoint.
	Data               map[string]interface{}
	CustomerConditions map[string]bool
}

// EvaluateWithPhotosResponse wraps the same EvaluationOutput the plain
// evaluate endpoint returns, plus the traceability the vendor integration
// needs: which Paradev call produced this result, and exactly what
// vision_signals it contributed (after invert + field_mapping), so an odd
// result in the field can be traced back to Paradev vs. the ruleset without
// guessing.
type EvaluateWithPhotosResponse struct {
	*EvaluationOutput
	ParadevTransactionID string             `json:"paradev_transaction_id"`
	VisionSignalsUsed    map[string]float64 `json:"vision_signals_used"`
}
