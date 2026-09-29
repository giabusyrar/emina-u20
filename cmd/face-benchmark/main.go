// Command face-benchmark measures a local, consented image set through the
// ai-worker, scores a face-architecture profile file against hand-collected
// vendor and rater labels, and fits a calibrated profile.
//
//	face-benchmark measure   --manifest m.csv --cache DIR --model-sha256 SHA
//	face-benchmark evaluate  --manifest m.csv --cache DIR --model-sha256 SHA --results r.csv --label-map l.json --profile p.json --out DIR
//	face-benchmark calibrate --manifest m.csv --cache DIR --model-sha256 SHA --results r.csv --label-map l.json --profile base.json --target T --out calibrated.json
//
// measure is the only subcommand that uses the network: it posts image
// bytes to VISION_AI_WORKER_URL (timeout FACE_MEASURE_TIMEOUT) and nowhere
// else, and prints that URL before the first upload. evaluate and calibrate
// read the cache and never call the worker. Nothing reads or writes the
// database: a calibrated profile is stored afterwards with facearch-profile
// upsert. Output names images by image_id, never by path.
//
// Exit codes: 0 done, 1 the run failed, 2 usage.
//
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Benchmark and calibration harness".
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/benchmark"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/packages/goplatform"
)

// deps are the outside world; tests replace them.
type deps struct {
	// worker returns the ai-worker client and the base URL images go to.
	worker func() (benchmark.Worker, string, error)
	// dhash hashes one image file (benchmark.DHashFile); imageID is only
	// for error messages, which must never carry the path.
	dhash func(path, imageID string) (uint64, error)
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, liveDeps()))
}

const usage = `usage:
  face-benchmark measure   --manifest <manifest.csv> --cache <dir> --model-sha256 <sha>
  face-benchmark evaluate  --manifest <manifest.csv> --cache <dir> --model-sha256 <sha>
                           --results <results.csv> --label-map <label-map.json> --profile <profile.json> --out <dir>
                           [--ece-bins N] [--min-consensus-raters N]
  face-benchmark calibrate --manifest <manifest.csv> --cache <dir> --model-sha256 <sha>
                           --results <results.csv> --label-map <label-map.json> --profile <base.json>
                           --target <perfectcorp|wardah|makeover|rater:<id>|rater-consensus> --out <calibrated.json>
                           [--held-out-fraction F] [--dedup-max-distance N] [--min-class-count N]
                           [--ece-bins N] [--seed N] [--min-consensus-raters N]
`

// common are the flags every subcommand takes.
type common struct {
	manifest, cache, model string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.manifest, "manifest", "", "manifest.csv")
	fs.StringVar(&c.cache, "cache", "", "cache directory")
	fs.StringVar(&c.model, "model-sha256", "", "sha256 of the ai-worker's face-landmarker manifest")
}

func (c *common) ok() bool { return c.manifest != "" && c.cache != "" && c.model != "" }

// labelled are the flags evaluate and calibrate share. The tunables
// start at benchmark.DefaultParams and are validated by Params.Validate.
type labelled struct {
	results, labelMap, profile, out string
	params                          benchmark.Params
}

func (l *labelled) register(fs *flag.FlagSet) {
	l.params = benchmark.DefaultParams()
	fs.StringVar(&l.results, "results", "", "results.csv")
	fs.StringVar(&l.labelMap, "label-map", "", "label-map.json")
	fs.StringVar(&l.profile, "profile", "", "profile document (JSON)")
	fs.StringVar(&l.out, "out", "", "output")
	fs.IntVar(&l.params.ECEBins, "ece-bins", l.params.ECEBins, "equal-width ECE bins")
	fs.IntVar(&l.params.MinConsensusRaters, "min-consensus-raters", l.params.MinConsensusRaters, "raters needed for a rater consensus")
}

func (l *labelled) ok() bool {
	return l.results != "" && l.labelMap != "" && l.profile != "" && l.out != ""
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) < 1 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c common
	c.register(fs)
	switch args[0] {
	case "measure":
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || !c.ok() {
			fmt.Fprint(stderr, usage)
			return 2
		}
		return measure(ctx, c, stdout, stderr, d)
	case "evaluate":
		var l labelled
		l.register(fs)
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || !c.ok() || !l.ok() {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if err := l.params.Validate(); err != nil {
			fmt.Fprintf(stderr, "invalid flags: %v\n%s", err, usage)
			return 2
		}
		return evaluate(c, l, stdout, stderr)
	case "calibrate":
		var l labelled
		l.register(fs)
		p := &l.params
		target := fs.String("target", "", "calibration target")
		fs.Float64Var(&p.HeldOutFraction, "held-out-fraction", p.HeldOutFraction, "share of subjects held out")
		fs.IntVar(&p.DedupMaxDistance, "dedup-max-distance", p.DedupMaxDistance, "dHash Hamming distance for near-duplicates")
		fs.IntVar(&p.MinClassCount, "min-class-count", p.MinClassCount, "training examples needed per class and label")
		fs.Uint64Var(&p.Seed, "seed", p.Seed, "subject split seed")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || !c.ok() || !l.ok() || *target == "" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if err := p.Validate(); err != nil {
			fmt.Fprintf(stderr, "invalid flags: %v\n%s", err, usage)
			return 2
		}
		if !benchmark.ValidTarget(*target) {
			fmt.Fprintf(stderr, "target must be perfectcorp, wardah, makeover, rater:<id> or %s, got %q\n%s", benchmark.TargetRaterConsensus, *target, usage)
			return 2
		}
		return calibrate(c, l, *target, stdout, stderr, d)
	}
	fmt.Fprint(stderr, usage)
	return 2
}

// redacted drops any user:password from a URL before it is printed.
func redacted(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable worker URL)"
	}
	return u.Redacted()
}

func measure(ctx context.Context, c common, stdout, stderr io.Writer, d deps) int {
	rows, err := benchmark.ReadManifest(c.manifest)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	w, workerURL, err := d.worker()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "measuring %d images with the ai-worker at %s (nothing is sent anywhere else)\n", len(rows), redacted(workerURL))
	sum, err := benchmark.Measure(ctx, rows, w, c.cache, c.model, stderr)
	if sum != nil {
		fmt.Fprintf(stdout, "catalogue %s: %d images, %d already cached, %d failed, recorded", sum.CatalogueVersion, sum.Images, sum.Cached, sum.Failed)
		for _, o := range benchmark.Outcomes {
			if n := sum.Recorded[o]; n > 0 {
				fmt.Fprintf(stdout, " %s %d", o, n)
			}
		}
		fmt.Fprintln(stdout)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if sum.Failed > 0 || sum.Recorded[benchmark.OutcomeUnavailable] > 0 {
		fmt.Fprintln(stderr, "some images have no final record; run measure again")
		return 1
	}
	return 0
}

// loaded is what evaluate and calibrate read before doing anything.
type loaded struct {
	cache   *benchmark.Cache
	profile *benchmark.ProfileFile
	items   []benchmark.Item
	labels  *benchmark.LabelSet
	raw     [][]byte // manifest, results, label map, profile bytes (for the run id)
}

func load(c common, l labelled) (*loaded, error) {
	rows, err := benchmark.ReadManifest(c.manifest)
	if err != nil {
		return nil, err
	}
	rawProfile, err := os.ReadFile(l.profile)
	if err != nil {
		return nil, err
	}
	doc, err := profile.Parse(rawProfile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", l.profile, err)
	}
	cache, err := benchmark.NewCache(c.cache, c.model, doc.CatalogueVersion)
	if err != nil {
		return nil, err
	}
	cat, err := cache.Catalogue()
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no measurements cached for model %s and catalogue %s; run measure first", c.model, doc.CatalogueVersion)
	}
	if err != nil {
		return nil, err
	}
	pf, err := benchmark.LoadProfileFile(l.profile, cat)
	if err != nil {
		return nil, err
	}
	results, err := benchmark.ReadResults(l.results, rows)
	if err != nil {
		return nil, err
	}
	lm, err := benchmark.ReadLabelMap(l.labelMap)
	if err != nil {
		return nil, err
	}
	targets, err := lm.Resolve(pf.Doc)
	if err != nil {
		return nil, err
	}
	labels, err := benchmark.BuildLabels(results, lm, targets, l.params.MinConsensusRaters)
	if err != nil {
		return nil, err
	}
	items, err := benchmark.LoadItems(rows, cache)
	if err != nil {
		return nil, err
	}
	var raw [][]byte
	for _, p := range []string{c.manifest, l.results, l.labelMap} {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		raw = append(raw, b)
	}
	return &loaded{cache: cache, profile: pf, items: items, labels: labels, raw: append(raw, rawProfile)}, nil
}

func evaluate(c common, l labelled, stdout, stderr io.Writer) int {
	in, err := load(c, l)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	rep, err := benchmark.Evaluate(benchmark.EvaluateInput{
		Items: in.items, Profile: in.profile, Labels: in.labels,
		ModelSHA256: in.cache.ModelSHA256(), CatalogueVersion: in.cache.CatalogueVersion(),
		Params: l.params,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.MkdirAll(l.out, 0o700); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := benchmark.WriteReport(l.out, rep); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s and %s in %s (%d images, %d source x attribute rows)\n",
		benchmark.ReportJSONName, benchmark.ReportMarkdownName, l.out, rep.Images, len(rep.Agreement))
	return 0
}

func calibrate(c common, l labelled, target string, stdout, stderr io.Writer, d deps) int {
	p := l.params
	in, err := load(c, l)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cat, err := in.cache.Catalogue()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// The run id covers the input files, every measurement and hash used,
	// and the settings: same inputs, same id, same profile.
	parts := append([][]byte{}, in.raw...)
	hashes := map[string]uint64{}
	for _, it := range in.items {
		if it.Outcome != benchmark.OutcomeMeasured {
			continue
		}
		h, err := d.dhash(it.Row.ImagePath, it.Row.ImageID)
		if err != nil {
			// DHashFile names the image id and never the path.
			fmt.Fprintln(stderr, err)
			return 1
		}
		hashes[it.Row.ImageID] = h
		m, err := json.Marshal(it.Response)
		if err != nil {
			fmt.Fprintf(stderr, "image %s: %v\n", it.Row.ImageID, err)
			return 1
		}
		parts = append(parts, m, []byte(fmt.Sprintf("%s %d", it.Row.ImageID, h)))
	}
	settings := fmt.Sprintf("model=%s catalogue=%s target=%s params=%+v", c.model, in.cache.CatalogueVersion(), target, p)
	runID := benchmark.RunID(append(parts, []byte(settings))...)
	res, err := benchmark.Calibrate(benchmark.CalibrateInput{
		Items: in.items, Hashes: hashes, Base: in.profile, Labels: in.labels, Target: target,
		Catalogue: cat, Params: p, RunID: runID,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(l.out, res.JSON, 0o600); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	h := res.HeldOut
	fmt.Fprintf(stdout, "wrote %s: calibrated, run %s, target %s\n", l.out, runID, target)
	fmt.Fprintf(stdout, "dedup removed %d of %d (%d across subjects); split %d/%d subjects, %d/%d images (train/held-out)\n",
		h.Dedup.Removed, h.Dedup.Considered, h.Dedup.CrossSubjectRemoved,
		h.Split.TrainSubjects, h.Split.HeldOutSubjects, h.Split.TrainImages, h.Split.HeldOutImages)
	if n := len(res.CrossSubjectDuplicates); n > 0 {
		fmt.Fprintf(stdout, "%d near-duplicate pair(s) belong to different subjects (likely manifest errors; check them):\n", n)
		for _, dup := range res.CrossSubjectDuplicates {
			fmt.Fprintf(stdout, "  %s ~ %s (distance %d)\n", dup.ImageID, dup.KeptID, dup.Distance)
		}
	}
	var keys []string
	for k := range h.Fitted {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f, b := h.Fitted[k], h.Base[k]
		fmt.Fprintf(stdout, "  %-28s held-out n %3d  exact %s (base %s)  macro-F1 %s (base %s)\n", k, f.N,
			fmtPtr(f.ExactAgreement), fmtPtr(b.ExactAgreement), fmtPtr(f.MacroF1), fmtPtr(b.MacroF1))
	}
	fmt.Fprintf(stdout, "store it with: facearch-profile upsert %s --brand <id|*> --app <id|*> --code <code>\n", l.out)
	return 0
}

func fmtPtr(x *float64) string {
	if x == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", *x)
}

func liveDeps() deps {
	return deps{
		worker: func() (benchmark.Worker, string, error) {
			goplatform.LoadDotEnv()
			workerURL := goplatform.First("VISION_AI_WORKER_URL")
			if workerURL == "" {
				workerURL = config.DefaultVisionAIWorkerURL
			}
			timeout, err := goplatform.Duration("FACE_MEASURE_TIMEOUT", config.DefaultFaceMeasureTimeout)
			if err != nil {
				return nil, "", err
			}
			w := benchmark.NewHTTPWorker(workerURL, timeout)
			return w, w.BaseURL(), nil
		},
		dhash: benchmark.DHashFile,
	}
}
