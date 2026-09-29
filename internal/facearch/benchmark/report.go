package benchmark

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Report file names written by WriteReport.
const (
	ReportJSONName     = "report.json"
	ReportMarkdownName = "report.md"
)

func num(x *float64) string {
	if x == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", *x)
}

func pct(x *float64) string {
	if x == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", 100**x)
}

// Markdown renders the report. Like report.json it holds no image paths
// and no image ids.
func (r *Report) Markdown() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# Face benchmark report\n\n")
	w("Profile `%s` (sha256 `%s`), calibration **%s**", r.Profile.Code, r.Profile.SHA256, r.Profile.CalibrationStatus)
	if r.Profile.BenchmarkRunID != nil {
		w(", run `%s`", *r.Profile.BenchmarkRunID)
	}
	if r.Profile.Target != nil {
		w(", target `%s`", *r.Profile.Target)
	}
	w(".\nModel `%s`, catalogue `%s`. %d images of %d subjects.\n\n",
		r.ModelSHA256, r.CatalogueVersion, r.Images, r.Subjects)
	if r.Profile.CalibrationStatus == profile.CalibrationCalibrated {
		w("**Figures on the image set this profile was calibrated on are in-sample.** If this report covers that set, the profile's numbers were fitted to the training subjects among these images, so agreement here overstates how it will do on new faces; only calibrate's `heldOutMetrics` are out-of-sample.\n\n")
	}
	p := r.Params
	w("Parameters used: held-out fraction %v, dHash max distance %d, min class count %d, ECE bins: %d, seed %d, min consensus raters %d (a rater consensus needs a strict majority of at least that many raters).\n\n",
		p.HeldOutFraction, p.DedupMaxDistance, p.MinClassCount, p.ECEBins, p.Seed, p.MinConsensusRaters)
	w("**Vendor labels are not ground truth.** They are other systems' opinions: ambiguous, and vendors disagree with each other. Agreement with a source measures resemblance to that source, not correctness. Read each ours-vs-source figure next to the source-vs-source kappa in the same row and in the source-vs-source table: agreement between two sources is the practical ceiling for our agreement with either. Source-vs-source covers every image both sources labelled, a different set from ours-vs-source (only images we measured and answered), so the figures are a reference point, not a bound. No image paths or ids appear in this report.\n\n")

	w("## Coverage\n\n")
	w("| slice | images |")
	for _, o := range Outcomes {
		w(" %s |", o)
	}
	w(" gate rejection rate |\n|---|---|")
	for range Outcomes {
		w("---|")
	}
	w("---|\n")
	for _, s := range SliceNames {
		c := r.Coverage[s]
		w("| %s | %d |", s, c.Images)
		for _, o := range Outcomes {
			w(" %d |", c.Outcomes[o])
		}
		w(" %s |\n", pct(c.GateRejectionRate))
	}
	w("\nNo-answer rate (insufficient_evidence, not_assessable or unavailable) over measured images:\n\n")
	w("| classifier / trait | %s |\n|---|%s\n", strings.Join(SliceNames, " | "), strings.Repeat("---|", len(SliceNames)))
	for _, k := range sortedKeys(r.Coverage[SliceAll].Answers) {
		w("| %s |", k)
		for _, s := range SliceNames {
			ac := r.Coverage[s].Answers[k]
			w(" %s of %d |", pct(ac.NoAnswerRate), ac.Measured)
		}
		w("\n")
	}

	w("\n## Agreement with each source\n\n")
	w("The last column is how this source agrees (Cohen's kappa) with each other source on the same attribute, over all images both labelled; it is not split by slice (`-`), and `n/a` means undefined.\n\n")
	w("Top-2 counts a match when the source's label is our primary or, for a blend, our secondary (for a trait: our label or its boundary alternative).\n\n")
	w("| attribute | source | slice | n | exact | top-2 | macro-F1 | kappa | ECE | source-vs-source kappa |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, a := range r.Agreement {
		for _, s := range SliceNames {
			m := a.Slices[s]
			ss := "-"
			if s == SliceAll {
				ss = r.sourcePairKappas(a.Attribute, a.Source)
			}
			w("| %s | %s | %s | %d | %s | %s | %s | %s | %s | %s |\n", a.Attribute, a.Source, s, m.N,
				pct(m.ExactAgreement), pct(m.Top2Agreement), num(m.MacroF1), num(m.Kappa), num(m.ECE), ss)
		}
	}
	w("\n| attribute | source | labelled | not measured | no answer from us | source abstained |\n|---|---|---|---|---|---|\n")
	for _, a := range r.Agreement {
		w("| %s | %s | %d | %d | %d | %d |\n", a.Attribute, a.Source, a.Labelled, a.NotMeasured, a.OursNoAnswer, a.Abstained)
	}

	w("\n## Confusion matrices (all images; rows: source, columns: ours)\n")
	for _, a := range r.Agreement {
		c := a.Slices[SliceAll].Confusion
		if len(c.Labels) == 0 {
			continue
		}
		w("\n### %s vs %s\n\n| source \\ ours | %s |\n|---|", a.Attribute, a.Source, strings.Join(c.Labels, " | "))
		for range c.Labels {
			w("---|")
		}
		w("\n")
		for i, l := range c.Labels {
			w("| %s |", l)
			for _, n := range c.Counts[i] {
				w(" %d |", n)
			}
			w("\n")
		}
	}

	w("\n## Source-vs-source agreement\n\n")
	if len(r.SourceAgreement) == 0 {
		w("No two sources labelled the same image.\n")
	} else {
		w("| attribute | source A | source B | n | exact | kappa |\n|---|---|---|---|---|---|\n")
		for _, p := range r.SourceAgreement {
			w("| %s | %s | %s | %d | %s | %s |\n", p.Attribute, p.SourceA, p.SourceB, p.N, pct(p.ExactAgreement), num(p.Kappa))
		}
	}
	if len(r.NoConsensus) > 0 {
		w("\nImages without a rater consensus, per attribute:")
		for _, k := range sortedKeys(r.NoConsensus) {
			w(" %s %d;", k, r.NoConsensus[k])
		}
		w("\n")
	}
	if len(r.UnmappedAttributes) > 0 {
		w("\nNot compared (attribute not in the label map), rows:")
		for _, k := range sortedKeys(r.UnmappedAttributes) {
			w(" %s %d;", k, r.UnmappedAttributes[k])
		}
		w("\n")
	}
	return b.String()
}

// sourcePairKappas lists the source-vs-source kappa of every pair holding
// source, or "n/a" when the source is in no pair.
func (r *Report) sourcePairKappas(attr, source string) string {
	var parts []string
	for _, p := range r.SourceAgreement {
		other := ""
		switch {
		case p.Attribute != attr:
			continue
		case p.SourceA == source:
			other = p.SourceB
		case p.SourceB == source:
			other = p.SourceA
		default:
			continue
		}
		parts = append(parts, fmt.Sprintf("vs %s %s (n=%d)", other, num(p.Kappa), p.N))
	}
	if len(parts) == 0 {
		return "n/a"
	}
	return strings.Join(parts, "; ")
}

// WriteReport writes report.json and report.md into dir.
func WriteReport(dir string, r *Report) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, ReportJSONName), append(raw, '\n')); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ReportMarkdownName), []byte(r.Markdown()))
}
