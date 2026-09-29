package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// MeasureSummary counts what one measure run did.
type MeasureSummary struct {
	CatalogueVersion string
	Images           int
	// Cached images were skipped: a final record already existed.
	Cached int
	// Recorded counts new records by outcome.
	Recorded map[string]int
	// Failed images got no record (unreadable file, worker unreachable,
	// unexpected status, a body that is not JSON); run measure again.
	Failed int
}

// Measure sends every manifest image without a final cache record to the
// worker and caches the answer. modelSHA256 is the model the operator
// expects (the ai-worker manifest's sha256); it keys the cache, and every
// 200 must report it, so a worker serving another model stops the run
// instead of filing its measurements under the wrong key. The run also
// stops if the worker's catalogue version changes mid-run. Progress lines
// name images by image_id only.
func Measure(ctx context.Context, rows []ManifestRow, w Worker, cacheDir, modelSHA256 string, log io.Writer) (*MeasureSummary, error) {
	catRaw, err := w.Catalogue(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalogue: %w", err)
	}
	var cat measureclient.Catalogue
	if err := json.Unmarshal(catRaw, &cat); err != nil {
		return nil, fmt.Errorf("catalogue: %w", err)
	}
	cache, err := NewCache(cacheDir, modelSHA256, cat.CatalogueVersion)
	if err != nil {
		return nil, err
	}
	if err := cache.PutCatalogue(catRaw); err != nil {
		return nil, err
	}
	sum := &MeasureSummary{CatalogueVersion: cat.CatalogueVersion, Images: len(rows), Recorded: map[string]int{}}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		image, err := os.ReadFile(row.ImagePath)
		if err != nil {
			sum.Failed++
			fmt.Fprintf(log, "%s: cannot read image file\n", row.ImageID)
			continue
		}
		digest := sha256.Sum256(image)
		imageSHA := hex.EncodeToString(digest[:])
		if rec, ok, err := cache.Get(imageSHA); err != nil {
			return sum, err
		} else if ok && rec.Final() {
			sum.Cached++
			continue
		}
		// The worker learns the bytes only: the file name sent is generic.
		status, body, err := w.Measure(ctx, image, "image"+strings.ToLower(filepath.Ext(row.ImagePath)))
		if err != nil {
			sum.Failed++
			fmt.Fprintf(log, "%s: %v\n", row.ImageID, err)
			continue
		}
		if !json.Valid(body) {
			sum.Failed++
			fmt.Fprintf(log, "%s: status %d with a body that is not JSON\n", row.ImageID, status)
			continue
		}
		if status == 200 {
			var resp measureclient.Response
			if err := json.Unmarshal(body, &resp); err != nil {
				sum.Failed++
				fmt.Fprintf(log, "%s: undecodable measurement: %v\n", row.ImageID, err)
				continue
			}
			if resp.CatalogueVersion != cat.CatalogueVersion {
				return sum, fmt.Errorf("%s: the worker measured with catalogue %s, but served catalogue %s at the start of this run; restart the run", row.ImageID, resp.CatalogueVersion, cat.CatalogueVersion)
			}
			if resp.Model.ManifestSha256 != modelSHA256 {
				return sum, fmt.Errorf("%s: the worker measured with model %s, expected %s; nothing was recorded for it", row.ImageID, resp.Model.ManifestSha256, modelSHA256)
			}
		}
		rec := Record{ImageSHA256: imageSHA, ModelSHA256: modelSHA256, CatalogueVersion: cat.CatalogueVersion, Status: status, Body: body}
		if err := cache.Put(rec); err != nil {
			if errors.Is(err, ErrFinalRecordExists) {
				// A final record appeared meanwhile: keep it.
				sum.Cached++
				continue
			}
			return sum, err
		}
		sum.Recorded[rec.Outcome()]++
	}
	return sum, nil
}
