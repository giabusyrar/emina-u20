package benchmark

import (
	"fmt"
	"math"
)

// LoAZ is the 97.5th percentile of the standard normal distribution: 95%
// limits of agreement are bias ± LoAZ·SD (Bland & Altman, "Statistical
// methods for assessing agreement between two methods of clinical
// measurement", Lancet 1986;327(8476):307-310). A property of the normal
// distribution, not a tolerance.
const LoAZ = 1.959963984540054

// MinPearsonPairs is the fewest pairs a Pearson r is reported for: with two
// pairs r is always ±1, so it carries no information.
const MinPearsonPairs = 3

// MeasureAgreement summarises paired values, ours against a vendor's, on the
// vendor's scale. Differences are ours − theirs. A statistic is nil when it
// is undefined: everything for N = 0, the SD and limits for N < 2, Pearson r
// for N < MinPearsonPairs or when either side has no variance. It judges
// nothing: there is no pass/fail.
type MeasureAgreement struct {
	N        int      `json:"n"`
	Bias     *float64 `json:"bias"`
	SDDiff   *float64 `json:"sdDiff"`
	LoALow   *float64 `json:"loaLow"`
	LoAHigh  *float64 `json:"loaHigh"`
	MAE      *float64 `json:"mae"`
	PearsonR *float64 `json:"pearsonR"`
}

// Agree computes the MeasureAgreement of paired finite values. It panics on
// slices of different lengths or a non-finite value: callers filter first.
func Agree(ours, theirs []float64) MeasureAgreement {
	if len(ours) != len(theirs) {
		panic(fmt.Sprintf("benchmark.Agree: %d values against %d", len(ours), len(theirs)))
	}
	for i := range ours {
		if math.IsNaN(ours[i]) || math.IsInf(ours[i], 0) || math.IsNaN(theirs[i]) || math.IsInf(theirs[i], 0) {
			panic(fmt.Sprintf("benchmark.Agree: pair %d is not finite", i))
		}
	}
	n := len(ours)
	a := MeasureAgreement{N: n}
	if n == 0 {
		return a
	}
	var sum, sumAbs float64
	for i := range ours {
		d := ours[i] - theirs[i]
		sum += d
		sumAbs += math.Abs(d)
	}
	bias := sum / float64(n)
	a.Bias, a.MAE = ptr(bias), ptr(sumAbs/float64(n))
	if n >= 2 {
		var ss float64
		for i := range ours {
			d := ours[i] - theirs[i] - bias
			ss += d * d
		}
		sd := math.Sqrt(ss / float64(n-1))
		a.SDDiff, a.LoALow, a.LoAHigh = ptr(sd), ptr(bias-LoAZ*sd), ptr(bias+LoAZ*sd)
	}
	if n >= MinPearsonPairs {
		if r, ok := pearsonR(ours, theirs); ok {
			a.PearsonR = ptr(r)
		}
	}
	return a
}

// constant reports whether every element of xs is equal to the first.
func constant(xs []float64) bool {
	if len(xs) == 0 {
		return true
	}
	first := xs[0]
	for i := 1; i < len(xs); i++ {
		if xs[i] != first {
			return false
		}
	}
	return true
}

// pearsonR is the sample correlation of x and y; false when either has no
// variance.
func pearsonR(x, y []float64) (float64, bool) {
	if constant(x) || constant(y) {
		return 0, false
	}
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(len(x))
	my /= float64(len(y))
	var sxy, sxx, syy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0, false
	}
	return sxy / math.Sqrt(sxx*syy), true
}
