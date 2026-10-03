package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
)

// Why an image is left out of one comparison, one reason per image, checked
// in this order (the vendor first: a photo not run through the vendor is the
// common case).
const (
	ExcludeNotMeasured       = "not_measured"
	ExcludeNotInCatalogue    = "not_in_cached_catalogue"
	ExcludeVendorAbsent      = "vendor_absent"
	ExcludeVendorUnparseable = "vendor_unparseable"
	ExcludeOursAbsent        = "ours_absent"
	ExcludeOursNotUsable     = "ours_not_usable"
)

// Report file names written by WriteMeasureReport.
const (
	MeasureReportJSONName     = "measure-report.json"
	MeasureReportMarkdownName = "measure-report.md"
)

// measureReportNote is repeated in every report: what the numbers are and
// are not.
const measureReportNote = "Agreement statistics only, no pass/fail: bias is ours minus the vendor's, " +
	"on the vendor's scale. Definitions differ (see each comparison's note), and any [1, x] " +
	"direction stated in a note is an assumption until checked against a real vendor screen."

// CompareInput is everything CompareMeasurements reads; it does no I/O.
type CompareInput struct {
	Items       []Item
	Results     []ResultRow
	Map         *MeasureMap
	Catalogue   *measureclient.Catalogue
	ModelSHA256 string
}

// MeasureReport is measure-report.json: aggregates only, never an image id.
type MeasureReport struct {
	Source           string             `json:"source"`
	CatalogueVersion string             `json:"catalogueVersion"`
	ModelSHA256      string             `json:"modelSha256"`
	Cite             string             `json:"cite"`
	Note             string             `json:"note"`
	Images           int                `json:"images"`
	Comparisons      []ComparisonReport `json:"comparisons"`
	Unused           map[string]string  `json:"unused"`
}

// ComparisonReport is one comparison's agreement, one MeasureAgreement per vendor
// component, in the vendor's order.
type ComparisonReport struct {
	Name       string             `json:"name"`
	Vendor     []string           `json:"vendor"`
	Ours       []string           `json:"ours"`
	Transform  string             `json:"transform"`
	Note       string             `json:"note"`
	Excluded   map[string]int     `json:"excluded"`
	Components []MeasureAgreement `json:"components"`
}

// CompareMeasurements pairs, per image, our cached measurements (on the
// vendor's scale) with the vendor's hand-recorded numbers for in.Map.Source.
func CompareMeasurements(in CompareInput) *MeasureReport {
	vendor := map[string]map[string]string{} // field -> image id -> raw
	for _, r := range in.Results {
		if r.Source != in.Map.Source || !strings.HasPrefix(r.Attribute, RatioPrefix) {
			continue
		}
		f := strings.TrimPrefix(r.Attribute, RatioPrefix)
		if vendor[f] == nil {
			vendor[f] = map[string]string{}
		}
		vendor[f][r.ImageID] = r.Value
	}
	rep := &MeasureReport{
		Source: in.Map.Source, CatalogueVersion: in.Map.CatalogueVersion, ModelSHA256: in.ModelSHA256,
		Cite: in.Map.Cite, Note: measureReportNote, Images: len(in.Items), Unused: in.Map.Unused,
	}
	if rep.Unused == nil {
		rep.Unused = map[string]string{}
	}
	for _, c := range in.Map.Comparisons {
		cr := ComparisonReport{Name: c.Name, Vendor: c.Vendor, Ours: c.Ours, Transform: c.Transform, Note: c.Note, Excluded: map[string]int{}}
		ours := make([][]float64, c.Components)
		theirs := make([][]float64, c.Components)
		if !allInCatalogue(in.Catalogue, c.Ours) {
			cr.Excluded[ExcludeNotInCatalogue] = len(in.Items)
		} else {
			for _, it := range in.Items {
				o, t, reason := pair(it, c, vendor)
				if reason != "" {
					cr.Excluded[reason]++
					continue
				}
				for k := range o {
					ours[k] = append(ours[k], o[k])
					theirs[k] = append(theirs[k], t[k])
				}
			}
		}
		for k := 0; k < c.Components; k++ {
			cr.Components = append(cr.Components, Agree(ours[k], theirs[k]))
		}
		rep.Comparisons = append(rep.Comparisons, cr)
	}
	return rep
}

func allInCatalogue(cat *measureclient.Catalogue, keys []string) bool {
	for _, k := range keys {
		if _, ok := cat.Entry(k); !ok {
			return false
		}
	}
	return true
}

// pair returns one image's values on the vendor's scale, or the exclusion
// reason.
func pair(it Item, c Comparison, vendor map[string]map[string]string) ([]float64, []float64, string) {
	if it.Outcome != OutcomeMeasured || it.Response == nil {
		return nil, nil, ExcludeNotMeasured
	}
	theirs := make([]float64, c.Components)
	for _, f := range c.Vendor {
		raw, ok := vendor[f][it.Row.ImageID]
		if !ok {
			return nil, nil, ExcludeVendorAbsent
		}
		v, ok := ParseVendorValue(raw, c.Components)
		if !ok {
			return nil, nil, ExcludeVendorUnparseable
		}
		for k := range v {
			theirs[k] += v[k] / float64(len(c.Vendor))
		}
	}
	for _, v := range theirs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, nil, ExcludeVendorUnparseable
		}
	}
	ours, reason := oursOnVendorScale(it.Response, c)
	if reason != "" {
		return nil, nil, reason
	}
	return ours, theirs, ""
}

// oursOnVendorScale applies the comparison's transform to our value(s).
func oursOnVendorScale(r *measureclient.Response, c Comparison) ([]float64, string) {
	ix := r.Index()
	scalar := func(k string) (float64, string) {
		o := ix.Observe(k)
		switch {
		case o.State == measureclient.StateMissing:
			return 0, ExcludeOursAbsent
		case !o.Usable():
			return 0, ExcludeOursNotUsable
		}
		return o.Value, ""
	}
	var out []float64
	switch c.Transform {
	case TransformIdentity, TransformInverse:
		v, reason := scalar(c.Ours[0])
		if reason != "" {
			return nil, reason
		}
		if c.Transform == TransformInverse {
			if v == 0 {
				return nil, ExcludeOursNotUsable
			}
			v = 1 / v
		}
		out = []float64{v}
	case TransformSharePercent:
		var sum float64
		vals := make([]float64, len(c.Ours))
		for i, k := range c.Ours {
			v, reason := scalar(k)
			if reason != "" {
				return nil, reason
			}
			vals[i], sum = v, sum+v
		}
		if sum <= 0 {
			return nil, ExcludeOursNotUsable
		}
		for _, v := range vals {
			out = append(out, 100*v/sum)
		}
	case TransformPercent:
		m, ok := find(r, c.Ours[0])
		switch {
		case !ok:
			return nil, ExcludeOursAbsent
		case m.Visibility == measureclient.VisibilityOccluded || m.Visibility == measureclient.VisibilityNotSupported ||
			!m.Value.IsList() || len(m.Value.List) != c.Components:
			return nil, ExcludeOursNotUsable
		}
		for _, v := range m.Value.List {
			out = append(out, 100*v)
		}
	default:
		panic(fmt.Sprintf("benchmark.CompareMeasurements: unknown transform %q (call MeasureMap.Check first)", c.Transform))
	}
	for _, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, ExcludeOursNotUsable
		}
	}
	return out, ""
}

func find(r *measureclient.Response, key string) (measureclient.Measurement, bool) {
	for _, m := range r.Measurements {
		if m.Key == key {
			return m, true
		}
	}
	return measureclient.Measurement{}, false
}

// WriteMeasureReport writes measure-report.json and measure-report.md into
// dir, each atomically.
func WriteMeasureReport(dir string, r *MeasureReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, MeasureReportJSONName), append(raw, '\n')); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, MeasureReportMarkdownName), []byte(measureMarkdown(r)))
}

func measureMarkdown(r *MeasureReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Measurements vs %s\n\n", r.Source)
	fmt.Fprintf(&b, "Catalogue %s, model %s, %d images. Map source: %s.\n\n", r.CatalogueVersion, r.ModelSHA256, r.Images, r.Cite)
	fmt.Fprintf(&b, "%s\n\n", r.Note)
	b.WriteString("| comparison | component | n | bias | SD of differences | 95% limits of agreement | MAE | Pearson r |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Comparisons {
		for k, a := range c.Components {
			loa := "—"
			if a.LoALow != nil {
				loa = fmt.Sprintf("%s to %s", fmtStat(a.LoALow), fmtStat(a.LoAHigh))
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %s | %s | %s | %s | %s |\n",
				c.Name, k+1, a.N, fmtStat(a.Bias), fmtStat(a.SDDiff), loa, fmtStat(a.MAE), fmtStat(a.PearsonR))
		}
	}
	b.WriteString("\n## Comparisons\n\n")
	if !hasValuePairs(r) {
		b.WriteString("No value pairs in any comparison: every image was left out, so the table above has no statistics. The exclusion counts below give the reasons.\n\n")
	}
	for _, c := range r.Comparisons {
		fmt.Fprintf(&b, "- **%s**: vendor %s, ours %s (%s). %s", c.Name, strings.Join(c.Vendor, " + "), strings.Join(c.Ours, ", "), c.Transform, c.Note)
		if len(c.Excluded) > 0 {
			var parts []string
			for _, k := range sortedKeys(c.Excluded) {
				parts = append(parts, fmt.Sprintf("%s %d", k, c.Excluded[k]))
			}
			fmt.Fprintf(&b, " Excluded images: %s.", strings.Join(parts, ", "))
			if c.Excluded[ExcludeNotInCatalogue] > 0 || c.Excluded[ExcludeOursAbsent] > 0 {
				b.WriteString(" A key missing from the cache: re-run `measure` with a fresh `--cache` to include it.")
			}
		}
		b.WriteString("\n")
	}
	if len(r.Unused) > 0 {
		b.WriteString("\n## Vendor fields not compared\n\n")
		for _, k := range sortedKeys(r.Unused) {
			fmt.Fprintf(&b, "- %s: %s\n", k, r.Unused[k])
		}
	}
	return b.String()
}

func hasValuePairs(r *MeasureReport) bool {
	for _, c := range r.Comparisons {
		for _, a := range c.Components {
			if a.N > 0 {
				return true
			}
		}
	}
	return false
}

func fmtStat(x *float64) string {
	if x == nil {
		return "—"
	}
	return fmt.Sprintf("%.4f", *x)
}
