package benchmark

// Defaults for the harness's tunables. None has a validated source: each
// is an Open literal of docs/superpowers/plans/2026-09-29-face-benchmark.md,
// documented in docs/FACE-BENCHMARK.md, overridable by a flag, and
// recorded in every report and calibrated profile it shaped.
const (
	// DefaultHeldOutFraction: share of subjects held out for evaluation.
	DefaultHeldOutFraction = 0.2
	// DefaultDedupMaxDistance: dHash Hamming distance (of 64 bits) at or
	// below which two images count as near-duplicates.
	DefaultDedupMaxDistance = 6
	// DefaultMinClassCount: training examples needed per class, prototype
	// measurement and trait label before calibrate fits a number.
	DefaultMinClassCount = 10
	// DefaultECEBins: equal-width confidence bins for ECE.
	DefaultECEBins = 10
	// DefaultSeed: seed of the subject split.
	DefaultSeed = 1
	// DefaultMinConsensusRaters: raters who must answer before their
	// strict majority counts as a rater consensus.
	DefaultMinConsensusRaters = 2
)

// Params are the tunables of one calibrate run.
type Params struct {
	HeldOutFraction    float64 `json:"heldOutFraction"`
	DedupMaxDistance   int     `json:"dedupMaxDistance"`
	MinClassCount      int     `json:"minClassCount"`
	ECEBins            int     `json:"eceBins"`
	Seed               uint64  `json:"seed"`
	MinConsensusRaters int     `json:"minConsensusRaters"`
}

// DefaultParams returns the defaults above.
func DefaultParams() Params {
	return Params{
		HeldOutFraction:    DefaultHeldOutFraction,
		DedupMaxDistance:   DefaultDedupMaxDistance,
		MinClassCount:      DefaultMinClassCount,
		ECEBins:            DefaultECEBins,
		Seed:               DefaultSeed,
		MinConsensusRaters: DefaultMinConsensusRaters,
	}
}
