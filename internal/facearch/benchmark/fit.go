package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// Example is one training image: its measurements and its target labels
// (our classifier or trait key -> label).
type Example struct {
	Index  measureclient.Index
	Labels map[string]string
}

// FitError lists everything that could not be fitted. Calibrate never
// mixes fitted and unfitted numbers in one calibrated profile: remove the
// listed classes, traits or rules from the base profile, or collect more
// labels.
type FitError struct {
	Problems []string
}

func (e *FitError) Error() string {
	return "cannot calibrate:\n  " + strings.Join(e.Problems, "\n  ")
}

// Fit is a fitted copy of the base document and the sources that cite the
// fitted numbers.
type Fit struct {
	Doc     *profile.Document
	Sources []profile.Source
	// Owners are the document paths whose numbers were all refitted
	// ("classifiers.<key>.classes.<label>", "traits.<key>"); base sources
	// for these paths no longer describe them.
	Owners []string
}

func copyDoc(d *profile.Document) (*profile.Document, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return profile.Parse(raw)
}

// meanSD returns the mean and the sample standard deviation (n-1), and
// whether the values vary at all (compared exactly: rounding in the mean
// would otherwise turn identical values into a tiny positive spread).
func meanSD(xs []float64) (float64, float64, bool) {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	varies := false
	for _, x := range xs {
		if x != xs[0] {
			varies = true
			break
		}
	}
	return mean, math.Sqrt(ss / float64(len(xs)-1)), varies
}

// FitProfile fits every classifier prototype and every trait threshold of
// base on the training examples, labelled by target.
//
// Classifiers: for each class and prototype measurement, centre is the
// mean and spread the sample standard deviation (n-1) of the measurement
// over training images labelled with that class and where it is usable.
// Weights, requires, blendMargin and minObservedWeight are kept. When every
// training value of a term is identical the spread would be zero, which a
// profile cannot carry; the term is refused rather than given an invented
// floor.
//
// Traits: rule order, labels, operators and default are kept; each
// condition's threshold is refitted, one condition at a time in rule
// order (a single coordinate pass, not a global optimum), to the midpoint
// between consecutive distinct training values that maximises macro-F1 of
// the whole trait against the target, then exact agreement; remaining ties
// go to the candidate nearest the base threshold, then the lower one.
// Training images where the trait is not assessable are left out.
//
// minClassCount applies to every class (usable values per prototype
// measurement) and to every label a trait can answer (assessable training
// images with that target label). Anything short of it, or with a zero
// spread, is a FitError: nothing is written.
func FitProfile(base *profile.Document, train []Example, minClassCount int, runID, target string) (*Fit, error) {
	if minClassCount < 2 {
		return nil, fmt.Errorf("minimum class count must be at least 2 (a spread needs two values), got %d", minClassCount)
	}
	doc, err := copyDoc(base)
	if err != nil {
		return nil, err
	}
	fit := &Fit{Doc: doc}
	var problems []string
	cite := "face-benchmark run " + runID

	for ci := range doc.Classifiers {
		spec := &doc.Classifiers[ci]
		for li := range spec.Classes {
			cl := &spec.Classes[li]
			owner := fmt.Sprintf("classifiers.%s.classes.%s", spec.Key, cl.Label)
			fit.Owners = append(fit.Owners, owner)
			for _, m := range cl.SortedKeys() {
				var xs []float64
				for _, ex := range train {
					if ex.Labels[spec.Key] != cl.Label {
						continue
					}
					if o := ex.Index.Observe(m); o.Usable() {
						xs = append(xs, o.Value)
					}
				}
				path := owner + ".prototype." + m
				if len(xs) < minClassCount {
					problems = append(problems, fmt.Sprintf("%s: %d usable training values labelled %s, need %d", path, len(xs), cl.Label, minClassCount))
					continue
				}
				mean, sd, varies := meanSD(xs)
				if !varies || !(sd > 0) || math.IsInf(sd, 0) {
					problems = append(problems, fmt.Sprintf("%s: the %d training values have no spread", path, len(xs)))
					continue
				}
				term := cl.Prototype[m]
				term.Centre, term.Spread = mean, sd
				cl.Prototype[m] = term
				fit.Sources = append(fit.Sources, profile.Source{Cite: cite, For: path,
					Note: fmt.Sprintf("centre = mean, spread = sample standard deviation of %s over %d training images labelled %s by %s; weight %v is not fitted: it is the base profile's (calibration.heldOutMetrics.baseProfileSha256)", m, len(xs), cl.Label, target, term.Weight)})
			}
		}
	}

	for ti := range doc.Traits {
		spec := &doc.Traits[ti]
		owner := "traits." + spec.Key
		fit.Owners = append(fit.Owners, owner)
		var xs []Example
		var truth []string
		for _, ex := range train {
			l, ok := ex.Labels[spec.Key]
			if !ok || traits.Evaluate(*spec, ex.Index).Status != traits.StatusAssessed {
				continue
			}
			xs = append(xs, ex)
			truth = append(truth, l)
		}
		counts := map[string]int{}
		for _, l := range truth {
			counts[l]++
		}
		answerable := map[string]bool{spec.Default: true}
		for _, r := range spec.Rules {
			answerable[r.Label] = true
		}
		short := false
		for _, l := range sortedKeys(answerable) {
			if counts[l] < minClassCount {
				problems = append(problems, fmt.Sprintf("%s: %d assessable training images labelled %s, need %d", owner, counts[l], l, minClassCount))
				short = true
			}
		}
		if short {
			continue
		}
		for ri := range spec.Rules {
			for ci := range spec.Rules[ri].When {
				cond := &spec.Rules[ri].When[ci]
				path := fmt.Sprintf("%s.rules.%d.when.%d", owner, ri, ci)
				values := map[float64]bool{}
				for _, ex := range xs {
					if o := ex.Index.Observe(cond.Measurement); o.Usable() {
						values[o.Value] = true
					}
				}
				sorted := make([]float64, 0, len(values))
				for v := range values {
					sorted = append(sorted, v)
				}
				sort.Float64s(sorted)
				if len(sorted) < 2 {
					problems = append(problems, fmt.Sprintf("%s: %s has fewer than 2 distinct training values", path, cond.Measurement))
					continue
				}
				baseValue := cond.Value
				bestF1, bestAcc, best := -1.0, -1.0, 0.0
				for i := 0; i+1 < len(sorted); i++ {
					cand := (sorted[i] + sorted[i+1]) / 2
					cond.Value = cand
					pred := make([]string, len(xs))
					for k, ex := range xs {
						pred[k] = traits.Evaluate(*spec, ex.Index).Label
					}
					f1, acc := *MacroF1(truth, pred), *Agreement(truth, pred)
					better := f1 > bestF1 || (f1 == bestF1 && acc > bestAcc) ||
						(f1 == bestF1 && acc == bestAcc && math.Abs(cand-baseValue) < math.Abs(best-baseValue))
					if better {
						bestF1, bestAcc, best = f1, acc, cand
					}
				}
				cond.Value = best
				fit.Sources = append(fit.Sources, profile.Source{Cite: cite, For: path,
					Note: fmt.Sprintf("%s %s threshold fitted on %d training images labelled by %s (training macro-F1 %.3f); rule order, labels and operators kept from the base profile", cond.Measurement, cond.Op, len(xs), target, bestF1)})
			}
		}
	}
	if len(problems) > 0 {
		return nil, &FitError{Problems: problems}
	}
	return fit, nil
}
