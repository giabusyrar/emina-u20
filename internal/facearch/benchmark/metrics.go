package benchmark

import (
	"fmt"
	"math"
	"sort"
)

// Pair is one image both sides answered.
type Pair struct {
	// Truth is the source's label, in our vocabulary.
	Truth string
	// Pred is our label: the classifier's primary, or the trait's label.
	Pred string
	// Top2 is our blend: primary plus, for a blend, secondary; for a trait,
	// the label plus the boundary alternative when boundaryUncertain. For a
	// single class or a certain trait it is just Pred, so top-2 agreement
	// equals exact agreement there.
	Top2 []string
	// Score is our normalised score for Pred (classifiers only; nil for
	// traits, which have no scores).
	Score *float64
}

// Confusion is a confusion matrix: rows are the source's labels, columns
// ours, both over the sorted union of labels either side used.
type Confusion struct {
	Labels []string `json:"labels"`
	Counts [][]int  `json:"counts"`
}

// Metrics compares our answers with one source. Every ratio is null when
// undefined (no pairs; kappa with chance agreement 1; ECE with no scores).
type Metrics struct {
	N              int      `json:"n"`
	ExactAgreement *float64 `json:"exactAgreement"`
	Top2Agreement  *float64 `json:"top2Agreement"`
	MacroF1        *float64 `json:"macroF1"`
	Kappa          *float64 `json:"kappa"`
	// ECE is the expected calibration error of Score as a probability that
	// Pred is the source's label, over ECEBins equal-width bins on [0, 1].
	// Scores are normalised over assessable classes, not probabilities;
	// ECE measures how far they are from behaving like ones.
	ECE       *float64  `json:"ece"`
	ECEBins   int       `json:"eceBins"`
	Confusion Confusion `json:"confusion"`
}

func ptr(x float64) *float64 { return &x }

// Agreement is the share of positions where a and b are equal.
func Agreement(a, b []string) *float64 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("benchmark.Agreement: %d vs %d labels", len(a), len(b)))
	}
	if len(a) == 0 {
		return nil
	}
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	return ptr(float64(same) / float64(len(a)))
}

// MacroF1 is the unweighted mean over the union of labels of
// F1 = 2TP / (2TP + FP + FN).
func MacroF1(truth, pred []string) *float64 {
	if len(truth) != len(pred) {
		panic(fmt.Sprintf("benchmark.MacroF1: %d vs %d labels", len(truth), len(pred)))
	}
	if len(truth) == 0 {
		return nil
	}
	tp, fp, fn := map[string]int{}, map[string]int{}, map[string]int{}
	for i := range truth {
		if truth[i] == pred[i] {
			tp[truth[i]]++
		} else {
			fn[truth[i]]++
			fp[pred[i]]++
		}
	}
	labels := unionLabels(truth, pred)
	var sum float64
	for _, l := range labels {
		sum += 2 * float64(tp[l]) / float64(2*tp[l]+fp[l]+fn[l])
	}
	return ptr(sum / float64(len(labels)))
}

// CohenKappa is (po - pe) / (1 - pe) for two raters' labels of the same
// items (Cohen 1960); nil when there are no items or pe is 1.
func CohenKappa(a, b []string) *float64 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("benchmark.CohenKappa: %d vs %d labels", len(a), len(b)))
	}
	if len(a) == 0 {
		return nil
	}
	n := float64(len(a))
	ca, cb := map[string]int{}, map[string]int{}
	same := 0
	for i := range a {
		ca[a[i]]++
		cb[b[i]]++
		if a[i] == b[i] {
			same++
		}
	}
	po := float64(same) / n
	// pe from integers with one division, so the result does not depend on
	// map iteration order.
	var cross int64
	for l, x := range ca {
		cross += int64(x) * int64(cb[l])
	}
	pe := float64(cross) / (n * n)
	if pe >= 1 {
		return nil
	}
	return ptr((po - pe) / (1 - pe))
}

// ECE is sum over bins of (n_bin / N) * |accuracy_bin - mean confidence_bin|
// (Guo et al. 2017 define the metric). Bin convention, chosen here and not
// taken from that paper: M equal-width bins on [0, 1], left-closed
// [k/M, (k+1)/M), so a score exactly on an edge goes to the upper bin, and a
// confidence of exactly 1 goes to the last bin. Panics on bins < 1 and on a
// score that is non-finite or outside [0, 1]: those are programming errors
// and must not reach report JSON.
func ECE(conf []float64, correct []bool, bins int) *float64 {
	if len(conf) != len(correct) {
		panic(fmt.Sprintf("benchmark.ECE: %d confidences vs %d outcomes", len(conf), len(correct)))
	}
	if bins < 1 {
		panic(fmt.Sprintf("benchmark.ECE: bins %d < 1", bins))
	}
	for _, c := range conf {
		if math.IsNaN(c) || math.IsInf(c, 0) || c < 0 || c > 1 {
			panic(fmt.Sprintf("benchmark.ECE: score %v outside [0, 1]", c))
		}
	}
	if len(conf) == 0 {
		return nil
	}
	n := make([]int, bins)
	sumConf := make([]float64, bins)
	hits := make([]int, bins)
	for i, c := range conf {
		b := int(c * float64(bins))
		if b >= bins {
			b = bins - 1
		}
		if b < 0 {
			b = 0
		}
		n[b]++
		sumConf[b] += c
		if correct[i] {
			hits[b]++
		}
	}
	var ece float64
	for b := range n {
		if n[b] == 0 {
			continue
		}
		acc := float64(hits[b]) / float64(n[b])
		mean := sumConf[b] / float64(n[b])
		ece += float64(n[b]) / float64(len(conf)) * math.Abs(acc-mean)
	}
	return ptr(ece)
}

// NewConfusion counts truth (rows) against pred (columns).
func NewConfusion(truth, pred []string) Confusion {
	if len(truth) != len(pred) {
		panic(fmt.Sprintf("benchmark.NewConfusion: %d vs %d labels", len(truth), len(pred)))
	}
	labels := unionLabels(truth, pred)
	at := make(map[string]int, len(labels))
	for i, l := range labels {
		at[l] = i
	}
	counts := make([][]int, len(labels))
	for i := range counts {
		counts[i] = make([]int, len(labels))
	}
	for i := range truth {
		counts[at[truth[i]]][at[pred[i]]]++
	}
	return Confusion{Labels: labels, Counts: counts}
}

// Compare computes every metric over pairs.
func Compare(pairs []Pair, eceBins int) Metrics {
	truth := make([]string, len(pairs))
	pred := make([]string, len(pairs))
	top2 := 0
	var conf []float64
	var correct []bool
	for i, p := range pairs {
		truth[i], pred[i] = p.Truth, p.Pred
		if len(p.Top2) > 2 {
			panic(fmt.Sprintf("benchmark.Compare: pair %d has %d top-2 labels", i, len(p.Top2)))
		}
		hit := p.Pred == p.Truth
		for _, l := range p.Top2 {
			if l == p.Truth {
				hit = true
			}
		}
		if hit {
			top2++
		}
		if p.Score != nil {
			conf = append(conf, *p.Score)
			correct = append(correct, p.Pred == p.Truth)
		}
	}
	m := Metrics{
		N:              len(pairs),
		ExactAgreement: Agreement(truth, pred),
		MacroF1:        MacroF1(truth, pred),
		Kappa:          CohenKappa(truth, pred),
		ECE:            ECE(conf, correct, eceBins),
		ECEBins:        eceBins,
		Confusion:      NewConfusion(truth, pred),
	}
	if len(pairs) > 0 {
		m.Top2Agreement = ptr(float64(top2) / float64(len(pairs)))
	}
	return m
}

func unionLabels(a, b []string) []string {
	set := map[string]bool{}
	for _, l := range a {
		set[l] = true
	}
	for _, l := range b {
		set[l] = true
	}
	out := make([]string, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
