package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// Item is one manifest image and its cached outcome.
type Item struct {
	Row     ManifestRow
	Outcome string
	// Response is the decoded worker answer; set only when Outcome is
	// OutcomeMeasured.
	Response *measureclient.Response
}

// LoadItems looks every manifest image up in the cache by the SHA-256 of
// its file.
func LoadItems(rows []ManifestRow, c *Cache) ([]Item, error) {
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		sha, err := FileSHA256(row.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot read image file: %w", row.ImageID, err)
		}
		rec, ok, err := c.Get(sha)
		if err != nil {
			return nil, err
		}
		it := Item{Row: row, Outcome: OutcomeNotMeasured}
		if ok {
			it.Outcome = rec.Outcome()
			if it.Outcome == OutcomeMeasured {
				if it.Response, err = rec.Response(); err != nil {
					return nil, fmt.Errorf("%s: %w", row.ImageID, err)
				}
			}
		}
		items = append(items, it)
	}
	return items, nil
}

// ProfileFile is a profile read from a file, as the endpoint would load it.
type ProfileFile struct {
	Entity *profile.FaceArchitectureProfile
	Doc    *profile.Document
	SHA256 string
}

// LoadProfileFile parses a profile document and validates it against the
// catalogue the cached measurements were taken with.
func LoadProfileFile(path string, cat *measureclient.Catalogue) (*ProfileFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := profile.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := profile.Validate(doc, cat); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	shaHex := hex.EncodeToString(sum[:])
	code := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return &ProfileFile{
		Entity: &profile.FaceArchitectureProfile{ID: "file:" + shaHex[:12], Code: code, Document: string(raw)},
		Doc:    doc,
		SHA256: shaHex,
	}, nil
}

// Answer is our answer for one classifier or trait on one image.
type Answer struct {
	// Status is the classifier or trait status (classify.Status*,
	// traits.Status*).
	Status string
	// Label is empty when there is no answer (insufficient_evidence,
	// not_assessable, unavailable).
	Label string
	Top2  []string
	Score *float64
}

// Answered reports whether the answer names a label.
func (a Answer) Answered() bool { return a.Label != "" }

// Answers runs the endpoint's own interpretation (facearch.Interpret) on a
// measured item and returns each classifier's and trait's answer.
func Answers(p *profile.FaceArchitectureProfile, doc *profile.Document, resp *measureclient.Response) map[Target]Answer {
	out := facearch.Interpret(p, doc, resp)
	answers := map[Target]Answer{}
	for key, r := range out.Classifications {
		a := Answer{Status: r.Status}
		if r.Status == classify.StatusSingle || r.Status == classify.StatusBlend {
			score, ok := r.Scores[r.Primary]
			if !ok || math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
				// Interpret returns no scores when they are not finite;
				// the metrics accept only finite scores in [0,1].
				a.Status = classify.StatusInsufficientEvidence
			} else {
				a.Label = r.Primary
				a.Top2 = []string{r.Primary}
				if r.Status == classify.StatusBlend {
					a.Top2 = append(a.Top2, r.Secondary)
				}
				a.Score = ptr(score)
			}
		}
		answers[Target{Kind: KindClassifier, Key: key}] = a
	}
	for key, r := range out.Traits {
		a := Answer{Status: r.Status}
		if r.Status == traits.StatusAssessed {
			a.Label = r.Label
			a.Top2 = []string{r.Label}
			if r.BoundaryUncertain && r.Alternative != "" {
				a.Top2 = append(a.Top2, r.Alternative)
			}
		}
		answers[Target{Kind: KindTrait, Key: key}] = a
	}
	return answers
}
