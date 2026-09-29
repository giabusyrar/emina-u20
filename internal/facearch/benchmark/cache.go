package benchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// Outcomes of one image, from its cache record.
const (
	// OutcomeMeasured: the worker answered 200.
	OutcomeMeasured = "measured"
	// OutcomeGateRejected: 422 quality_gates_failed (pose or resolution).
	OutcomeGateRejected = "gate_rejected"
	// OutcomeFaceCount: 422 face_count (zero or several faces).
	OutcomeFaceCount = "face_count"
	// OutcomeRejectedOther: any other 422 (for example FastAPI's own
	// request validation, whose detail is a list).
	OutcomeRejectedOther = "rejected_other"
	// OutcomeUnreadable: 400 (empty or undecodable image).
	OutcomeUnreadable = "unreadable"
	// OutcomeUnavailable: 503 (the worker had no usable model).
	OutcomeUnavailable = "unavailable"
	// OutcomeNotMeasured: no cache record for this image, model and
	// catalogue.
	OutcomeNotMeasured = "not_measured"
)

// Outcomes lists every outcome in report order.
var Outcomes = []string{OutcomeMeasured, OutcomeGateRejected, OutcomeFaceCount, OutcomeRejectedOther,
	OutcomeUnreadable, OutcomeUnavailable, OutcomeNotMeasured}

// Record is one cached worker answer. Body is the worker's JSON as
// received (re-indented by encoding/json, numbers unchanged). A 200 body
// is only ever read back through json.Unmarshal into
// measureclient.Response, the same decoding the endpoint uses, so the
// harness never builds a response by hand.
type Record struct {
	ImageSHA256      string          `json:"imageSha256"`
	ModelSHA256      string          `json:"modelSha256"`
	CatalogueVersion string          `json:"catalogueVersion"`
	Status           int             `json:"status"`
	Body             json.RawMessage `json:"body"`
}

// Final reports whether measure should skip this image next time. A 503
// says the worker had no model, not something about the image, so it is
// recorded (evaluate counts it) but measured again on the next run.
func (r *Record) Final() bool { return r.Status != 503 }

// Outcome classifies the record.
func (r *Record) Outcome() string {
	switch r.Status {
	case 200:
		return OutcomeMeasured
	case 400:
		return OutcomeUnreadable
	case 503:
		return OutcomeUnavailable
	case 422:
		var body struct {
			Detail json.RawMessage `json:"detail"`
		}
		var detail struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(r.Body, &body) == nil && json.Unmarshal(body.Detail, &detail) == nil {
			switch detail.Code {
			case "quality_gates_failed":
				return OutcomeGateRejected
			case "face_count":
				return OutcomeFaceCount
			}
		}
	}
	return OutcomeRejectedOther
}

// Response decodes a 200 record's body and checks it belongs to the
// record's model and catalogue.
func (r *Record) Response() (*measureclient.Response, error) {
	if r.Status != 200 {
		return nil, fmt.Errorf("image %s: cached status %d has no measurements", r.ImageSHA256, r.Status)
	}
	var resp measureclient.Response
	if err := json.Unmarshal(r.Body, &resp); err != nil {
		return nil, fmt.Errorf("image %s: cached body: %w", r.ImageSHA256, err)
	}
	if resp.CatalogueVersion != r.CatalogueVersion {
		return nil, fmt.Errorf("image %s: cached body is catalogue %s, record says %s", r.ImageSHA256, resp.CatalogueVersion, r.CatalogueVersion)
	}
	if resp.Model.ManifestSha256 != r.ModelSHA256 {
		return nil, fmt.Errorf("image %s: cached body is model %s, record says %s", r.ImageSHA256, resp.Model.ManifestSha256, r.ModelSHA256)
	}
	return &resp, nil
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Cache holds records for one model (manifest SHA-256) and one catalogue
// version, keyed by image SHA-256:
// <dir>/<modelSha256>/<catalogue slug>/<imageSha256>.json, plus
// catalogue.json, the catalogue the worker served when they were measured.
type Cache struct {
	dir              string
	modelSHA256      string
	catalogueVersion string
}

// NewCache opens (without creating) the cache for one model and catalogue.
func NewCache(dir, modelSHA256, catalogueVersion string) (*Cache, error) {
	if dir == "" {
		return nil, errors.New("cache directory is required")
	}
	if !sha256Hex.MatchString(modelSHA256) {
		return nil, fmt.Errorf("model SHA-256 must be 64 lowercase hex digits (the ai-worker manifest's sha256), got %q", modelSHA256)
	}
	if strings.TrimSpace(catalogueVersion) == "" {
		return nil, errors.New("catalogue version is required")
	}
	return &Cache{dir: dir, modelSHA256: modelSHA256, catalogueVersion: catalogueVersion}, nil
}

// ModelSHA256 is the model this cache holds records for.
func (c *Cache) ModelSHA256() string { return c.modelSHA256 }

// CatalogueVersion is the catalogue this cache holds records for.
func (c *Cache) CatalogueVersion() string { return c.catalogueVersion }

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			return r
		}
		return '_'
	}, s)
}

func (c *Cache) base() string {
	return filepath.Join(c.dir, c.modelSHA256, slug(c.catalogueVersion))
}

// FileSHA256 is the hex SHA-256 of a file's bytes.
func FileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Get returns the record for an image, or ok false when there is none.
func (c *Cache) Get(imageSHA256 string) (*Record, bool, error) {
	raw, err := os.ReadFile(filepath.Join(c.base(), imageSHA256+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, false, fmt.Errorf("cache record %s: %w", imageSHA256, err)
	}
	if r.ImageSHA256 != imageSHA256 || r.ModelSHA256 != c.modelSHA256 || r.CatalogueVersion != c.catalogueVersion {
		return nil, false, fmt.Errorf("cache record %s: holds image %s, model %s, catalogue %s", imageSHA256, r.ImageSHA256, r.ModelSHA256, r.CatalogueVersion)
	}
	return &r, true, nil
}

// Put writes a record (replacing any earlier one) through a temporary
// file, so an interrupted run never leaves half a record.
func (c *Cache) Put(r Record) error {
	if r.ModelSHA256 != c.modelSHA256 || r.CatalogueVersion != c.catalogueVersion {
		return fmt.Errorf("record for model %s catalogue %s does not belong in cache for %s %s", r.ModelSHA256, r.CatalogueVersion, c.modelSHA256, c.catalogueVersion)
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(c.base(), r.ImageSHA256+".json"), raw)
}

// PutCatalogue stores the worker's catalogue body. If one is already
// stored, the two must decode to the same catalogue: a definition change
// without a version bump would make cached values incomparable.
func (c *Cache) PutCatalogue(raw []byte) error {
	var cat measureclient.Catalogue
	if err := json.Unmarshal(raw, &cat); err != nil {
		return fmt.Errorf("catalogue: %w", err)
	}
	if cat.CatalogueVersion != c.catalogueVersion {
		return fmt.Errorf("catalogue is %s, cache is for %s", cat.CatalogueVersion, c.catalogueVersion)
	}
	if old, err := c.Catalogue(); err == nil {
		if !reflect.DeepEqual(old, &cat) {
			return fmt.Errorf("the worker's catalogue %s differs from the one cached earlier under the same version; its definitions changed without a version bump", c.catalogueVersion)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return fmt.Errorf("catalogue: %w", err)
	}
	return writeAtomic(filepath.Join(c.base(), "catalogue.json"), pretty.Bytes())
}

// Catalogue reads the stored catalogue (fs.ErrNotExist when measure has
// not run for this model and catalogue).
func (c *Cache) Catalogue() (*measureclient.Catalogue, error) {
	raw, err := os.ReadFile(filepath.Join(c.base(), "catalogue.json"))
	if err != nil {
		return nil, err
	}
	var cat measureclient.Catalogue
	if err := json.Unmarshal(raw, &cat); err != nil {
		return nil, fmt.Errorf("cached catalogue: %w", err)
	}
	return &cat, nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
