package benchmark

import (
	"fmt"
	"math"
)

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
	// DefaultMinConsensusRaters: raters who must agree on the same label.
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

// DHashBits is the length of a dHash in bits (an 8x9 grayscale grid gives
// 8x8 horizontal-gradient comparisons: Neal Krawetz, "Kind of Like That",
// 2013). It bounds any Hamming distance between two hashes.
const DHashBits = 64

// Validate rejects tunables that would make a run meaningless or panic. Every
// tunable is overridable by a flag, so the message names the field and the
// value it was given.
func (p Params) Validate() error {
	if math.IsNaN(p.HeldOutFraction) || p.HeldOutFraction <= 0 || p.HeldOutFraction >= 1 {
		return fmt.Errorf("HeldOutFraction must be in (0,1), got %v", p.HeldOutFraction)
	}
	if p.DedupMaxDistance < 0 || p.DedupMaxDistance > DHashBits {
		return fmt.Errorf("DedupMaxDistance must be in 0..%d, got %d", DHashBits, p.DedupMaxDistance)
	}
	if p.MinClassCount < 1 {
		return fmt.Errorf("MinClassCount must be at least 1, got %d", p.MinClassCount)
	}
	if p.ECEBins < 1 {
		return fmt.Errorf("ECEBins must be at least 1, got %d", p.ECEBins)
	}
	if p.MinConsensusRaters < 1 {
		return fmt.Errorf("MinConsensusRaters must be at least 1, got %d", p.MinConsensusRaters)
	}
	return nil
}
