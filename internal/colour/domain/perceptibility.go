package domain

// Perceptibility of a CIEDE2000 colour difference to a human observer. These
// are properties of human vision, not policy, so they are fixed here and never
// configured (AGENTS.md, "What this rule does not cover": JND 1.0,
// close-inspection 2.0, at-a-glance 5.0). A brand's own match tolerance is
// policy and lives in config (skinMatch.maxDeltaE00).
const (
	// DeltaE00JND: below this the difference is not perceptible (just
	// noticeable difference).
	DeltaE00JND = 1.0
	// DeltaE00CloseInspection: below this a difference is visible only on
	// close inspection.
	DeltaE00CloseInspection = 2.0
	// DeltaE00AtAGlance: from this up two colours read as different at a glance.
	DeltaE00AtAGlance = 5.0
)

// Perceptibility bands reported with a ΔE00.
const (
	BandNotPerceptible  = "not_perceptible"  // ΔE00 < 1.0
	BandCloseInspection = "close_inspection" // 1.0 ≤ ΔE00 < 2.0
	BandNoticeable      = "noticeable"       // 2.0 ≤ ΔE00 < 5.0
	BandAtAGlance       = "at_a_glance"      // ΔE00 ≥ 5.0
)

// PerceptibilityBand names how visible a CIEDE2000 difference is.
func PerceptibilityBand(dE float64) string {
	switch {
	case dE < DeltaE00JND:
		return BandNotPerceptible
	case dE < DeltaE00CloseInspection:
		return BandCloseInspection
	case dE < DeltaE00AtAGlance:
		return BandNoticeable
	default:
		return BandAtAGlance
	}
}
