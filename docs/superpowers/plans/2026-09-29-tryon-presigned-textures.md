# Tryon Presigned Textures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tryon stores object keys instead of public URLs; core-engine presigns short-lived links on every `GET /shade-assets/:id`, so the R2 bucket can go private.

**Architecture:** ai-worker's tryon callback sends `colorMapKey`/`alphaMapKey`/`finishMapKey`. core-engine stores them in new `*_key` columns and, on read, signs each key locally with `aws-sdk-go-v2`'s presign client (no network call). A one-off command (`cmd/shade-asset-keys`) converts existing URL rows to keys and drops the old columns. The web contract (`colorMapUrl` etc. in the response) is unchanged.

**Tech Stack:** Go 1.25, Echo v4, GORM v1.25 (Postgres), `github.com/aws/aws-sdk-go-v2` (`service/s3`, `credentials`); Python 3.12 FastAPI ai-worker, pytest.

**Spec:** `docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md`

## Global Constraints

- Env names shared with ai-worker, unchanged: `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_REGION` (default `auto`). New: `S3_PRESIGN_TTL`, default `15m`.
- "S3 configured" means bucket, access key and secret are all non-empty (same test as ai-worker's `s3_configured`).
- S3 is optional for core-engine: it must boot without it. Unconfigured → `GET /shade-assets/:id` answers 503 `object storage not configured`. Never a fake or unsigned URL.
- `GET /shade-assets/:id` response shape stays `{"success": true, "asset": {"id","shadeId","colorMapUrl","alphaMapUrl","finishMapUrl"?,"createdAt"}}`. Stored keys are never serialized (`json:"-"`).
- Error bodies never echo internal errors; log them instead.
- `S3_PUBLIC_URL` is read only by `cmd/shade-asset-keys`, to recognise legacy rows. ai-worker drops it.
- No hardcoded hosts/buckets/prefixes. Test fixtures use `example` hosts.
- AGENTS.md: never `git push`. Credentials in `.env` are never printed.
- Go formatting: `node scripts/go-tools.mjs fmt` (strips CRLF). Standalone build: `npm run build:check`.

## File Map

| File | Responsibility |
|---|---|
| `apps/core-engine/internal/config/config.go` (+ `config_test.go`) | S3 fields, `S3PresignTTL`, defaults |
| `apps/core-engine/internal/match/adapters/objectstore/presigner.go` (+ `_test.go`) | `Presigner` interface, `Config`, `New`, S3 presign implementation |
| `apps/core-engine/internal/match/entity/entity.go` | `ShadeAsset` key fields |
| `apps/core-engine/internal/match/adapters/controllers/match_handler.go` (+ tests) | Callback takes keys; `GetShadeAsset` signs |
| `apps/core-engine/internal/match/repository/match_repo_test.go` | Field rename in existing test |
| `apps/core-engine/cmd/server/main.go` | Build presigner, pass to handler |
| `apps/core-engine/cmd/shade-asset-keys/{main.go,keys.go,store.go}` (+ tests) | Legacy URL → key migration |
| `apps/ai-worker/ai_worker/tryon/services/storage.py`, `tryon/routers/extraction.py` | Return / send keys |
| `apps/ai-worker/ai_worker/core/{storage.py,settings.py}` | Remove `public_url`, `s3_public_url` |
| `apps/ai-worker/tests/core/{test_storage.py,test_settings.py}`, `tests/tryon/test_extraction_endpoint.py` | Tests |
| `docs/OBJECT-STORAGE.md` (new), `docs/SHARED-DATABASE.md`, the spec | Operator steps, column drop record |

---

### Task 0: Workspace and lanes

**Files:** `docs/LANES.md` (claims table only, in the main checkout)

- [ ] **Step 1: Claim lanes.** In `C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull-core/docs/LANES.md`, replace this session's docs-only row with one row covering: `core-match` (`apps/core-engine/internal/match/`), `apps/core-engine/internal/config/`, `apps/core-engine/cmd/server/main.go`, `apps/core-engine/cmd/shade-asset-keys/`, `apps/core-engine/go.mod`, `apps/core-engine/go.sum`, `ai-worker-core` (`ai_worker/core/storage.py`, `ai_worker/core/settings.py`, `tests/core/test_storage.py`, `tests/core/test_settings.py`), `ai-worker-tryon`, `docs/OBJECT-STORAGE.md`, `docs/SHARED-DATABASE.md`, this plan and its spec. Note "work happens on branch `feat/tryon-presigned-textures` in `seagull-core-worktrees/tryon-presigned-textures`". Do not touch any other session's row.

- [ ] **Step 2: Create the worktree.**

```bash
git -C C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull-core worktree add ../seagull-core-worktrees/tryon-presigned-textures -b feat/tryon-presigned-textures main
```

All later tasks run inside `C:/Users/GiaBusyraRabbani/Documents/Paragon/seagull-core-worktrees/tryon-presigned-textures` (called "the worktree" below). The main checkout holds uncommitted `S3_PUBLIC_URL` edits to four ai-worker files; they are superseded by Task 4 and are dealt with in Task 7, not here.

- [ ] **Step 3: Baseline.** In the worktree: `npm run test:go` → PASS; `pytest apps/ai-worker/tests/core -q` → PASS. Record any pre-existing failure before changing anything.

---

### Task 1: core-engine S3 config

**Files:**
- Modify: `apps/core-engine/internal/config/config.go`
- Test: `apps/core-engine/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` fields `S3Endpoint, S3Bucket, S3AccessKeyID, S3SecretAccessKey, S3Region string`, `S3PresignTTL time.Duration`; constants `config.DefaultS3Region = "auto"`, `config.DefaultS3PresignTTL = 15 * time.Minute`.

- [ ] **Step 1: Write the failing tests.** In `config_test.go`, add `"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_REGION", "S3_PRESIGN_TTL"` to the `tunables` slice (so `setBaseEnv` blanks them), then append:

```go
func TestFromEnv_S3IsOptionalWithDefaults(t *testing.T) {
	setBaseEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.S3Bucket != "" || cfg.S3AccessKeyID != "" || cfg.S3SecretAccessKey != "" || cfg.S3Endpoint != "" {
		t.Fatalf("S3 fields should be empty when unset, got %+v", cfg)
	}
	if cfg.S3Region != config.DefaultS3Region || cfg.S3PresignTTL != config.DefaultS3PresignTTL {
		t.Fatalf("S3Region=%q S3PresignTTL=%v, want defaults", cfg.S3Region, cfg.S3PresignTTL)
	}
}

func TestFromEnv_S3Values(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("S3_ENDPOINT", "https://acct.example-storage.com")
	t.Setenv("S3_BUCKET", "bkt")
	t.Setenv("S3_ACCESS_KEY_ID", "id")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	t.Setenv("S3_REGION", "ap-southeast-1")
	t.Setenv("S3_PRESIGN_TTL", "90s")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.S3Endpoint != "https://acct.example-storage.com" || cfg.S3Bucket != "bkt" ||
		cfg.S3AccessKeyID != "id" || cfg.S3SecretAccessKey != "secret" ||
		cfg.S3Region != "ap-southeast-1" || cfg.S3PresignTTL != 90*time.Second {
		t.Fatalf("got %+v", cfg)
	}
}

func TestFromEnv_S3PresignTTLInvalidIsAnError(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("S3_PRESIGN_TTL", "soon")
	if _, err := config.FromEnv(); err == nil {
		t.Fatal("want an error for S3_PRESIGN_TTL=soon")
	}
}
```

- [ ] **Step 2: Run to verify failure.** `go test ./apps/core-engine/internal/config/...` → FAIL (`cfg.S3Bucket undefined`).

- [ ] **Step 3: Implement.** In `config.go`, add after `DefaultFaceMeasureTimeout`:

```go
// Object storage for tryon shade textures. The S3_* names are the ones
// ai-worker already reads, so one .env serves both services.
const (
	// DefaultS3Region matches ai-worker's S3_REGION default; Cloudflare R2
	// requires "auto".
	DefaultS3Region = "auto"
	// DefaultS3PresignTTL is how long a shade texture link stays valid.
	// Product policy, not a measurement: the web app fetches the shade asset
	// and loads its images immediately and never keeps the URL, so minutes
	// suffice. docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md.
	DefaultS3PresignTTL = 15 * time.Minute
)
```

Add to `Config` after `TryonEngineURL`:

```go
	// S3* reach the bucket holding tryon shade textures. All optional:
	// without bucket and keys, shade asset links are unavailable (503).
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3PresignTTL      time.Duration
```

In `FromEnv`, add to the struct literal:

```go
		S3Endpoint:           goplatform.First("S3_ENDPOINT"),
		S3Bucket:             goplatform.First("S3_BUCKET"),
		S3AccessKeyID:        goplatform.First("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:    goplatform.First("S3_SECRET_ACCESS_KEY"),
		S3Region:             firstOr("S3_REGION", DefaultS3Region),
```

and to the duration table: `{"S3_PRESIGN_TTL", DefaultS3PresignTTL, &cfg.S3PresignTTL},`.

- [ ] **Step 4: Run tests.** `go test ./apps/core-engine/internal/config/...` → PASS.

- [ ] **Step 5: Commit.**

```bash
node scripts/go-tools.mjs fmt
git add apps/core-engine/internal/config
git commit -m "feat(core-engine): S3 config and S3_PRESIGN_TTL for shade texture links"
```

---

### Task 2: objectstore presigner

**Files:**
- Create: `apps/core-engine/internal/match/adapters/objectstore/presigner.go`
- Test: `apps/core-engine/internal/match/adapters/objectstore/presigner_test.go`
- Modify: `apps/core-engine/go.mod`, `apps/core-engine/go.sum`

**Interfaces:**
- Produces:

```go
package objectstore
type Presigner interface { Presign(ctx context.Context, key string) (string, error) }
type Config struct { Endpoint, Bucket, AccessKeyID, SecretAccessKey, Region string; TTL time.Duration }
func (c Config) Configured() bool
func New(c Config) (Presigner, error) // (nil, nil) when !c.Configured()
```

- [ ] **Step 1: Add the dependency.**

```bash
cd apps/core-engine && go get github.com/aws/aws-sdk-go-v2 github.com/aws/aws-sdk-go-v2/credentials github.com/aws/aws-sdk-go-v2/service/s3
```

- [ ] **Step 2: Write the failing tests** (`presigner_test.go`):

```go
package objectstore

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		Endpoint:        "https://acct.example-storage.com",
		Bucket:          "bkt",
		AccessKeyID:     "id",
		SecretAccessKey: "secret",
		Region:          "auto",
		TTL:             15 * time.Minute,
	}
}

func TestPresignAddressesBucketAndKeyAndCarriesTTL(t *testing.T) {
	p, err := New(testConfig())
	if err != nil || p == nil {
		t.Fatalf("New = %v, %v", p, err)
	}
	raw, err := p.Presign(context.Background(), "shades/9/color.png")
	if err != nil {
		t.Fatalf("Presign: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	if u.Host != "acct.example-storage.com" || u.Path != "/bkt/shades/9/color.png" {
		t.Fatalf("host=%q path=%q, want the endpoint host and /bucket/key", u.Host, u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Expires") != "900" {
		t.Fatalf("X-Amz-Expires = %q, want 900", q.Get("X-Amz-Expires"))
	}
	if q.Get("X-Amz-Signature") == "" {
		t.Fatal("URL is not signed")
	}
}

func TestNewReturnsNilWhenUnconfigured(t *testing.T) {
	c := testConfig()
	c.SecretAccessKey = ""
	p, err := New(c)
	if err != nil || p != nil {
		t.Fatalf("New = %v, %v; want nil, nil", p, err)
	}
}

func TestNewRejectsNonPositiveTTL(t *testing.T) {
	c := testConfig()
	c.TTL = 0
	if _, err := New(c); err == nil {
		t.Fatal("want an error for TTL=0")
	}
}
```

- [ ] **Step 3: Run to verify failure.** `go test ./apps/core-engine/internal/match/adapters/objectstore/...` → FAIL (undefined `New`).

- [ ] **Step 4: Implement** (`presigner.go`):

```go
// Package objectstore signs short-lived download links for objects in the
// S3-compatible bucket that holds tryon shade textures. Signing is local
// HMAC work: no request reaches the storage provider until the browser
// follows the link.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Presigner turns an object key into a time-limited GET URL.
type Presigner interface {
	Presign(ctx context.Context, key string) (string, error)
}

// Config mirrors the S3_* environment shared with ai-worker.
type Config struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	TTL             time.Duration
}

// Configured uses the same test as ai-worker's settings.s3_configured.
func (c Config) Configured() bool {
	return c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
}

// New returns (nil, nil) when c is not configured, so callers can tell
// "no object storage" apart from a broken configuration.
func New(c Config) (Presigner, error) {
	if !c.Configured() {
		return nil, nil
	}
	if c.TTL <= 0 {
		return nil, errors.New("objectstore: TTL must be positive")
	}
	client := s3.NewFromConfig(aws.Config{
		Region:      c.Region,
		Credentials: credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, ""),
	}, func(o *s3.Options) {
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
		}
		// Path-style (/bucket/key): R2 and most S3-compatible providers
		// accept it, and it keeps the endpoint host unchanged.
		o.UsePathStyle = true
	})
	return &s3Presigner{client: s3.NewPresignClient(client), bucket: c.Bucket, ttl: c.TTL}, nil
}

type s3Presigner struct {
	client *s3.PresignClient
	bucket string
	ttl    time.Duration
}

func (p *s3Presigner) Presign(ctx context.Context, key string) (string, error) {
	req, err := p.client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(p.ttl))
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", key, err)
	}
	return req.URL, nil
}
```

- [ ] **Step 5: Run tests.** `go test ./apps/core-engine/internal/match/adapters/objectstore/...` → PASS. Then `npm run build:check` → PASS (standalone module build picks up the new requires).

- [ ] **Step 6: Commit.**

```bash
node scripts/go-tools.mjs fmt
git add apps/core-engine/internal/match/adapters/objectstore apps/core-engine/go.mod apps/core-engine/go.sum
git commit -m "feat(core-engine): objectstore presigner for shade texture links"
```

---

### Task 3: Shade assets store keys; GET signs them

**Files:**
- Modify: `apps/core-engine/internal/match/entity/entity.go:90-97`
- Modify: `apps/core-engine/internal/match/adapters/controllers/match_handler.go` (struct, `NewMatchHandler`, `GetShadeAsset`, `ExtractionCallback`)
- Modify: `apps/core-engine/cmd/server/main.go:110-114`
- Test: `apps/core-engine/internal/match/adapters/controllers/match_handler_test.go`, `envelope_test.go:22`, `apps/core-engine/internal/match/repository/match_repo_test.go:55-80`

**Interfaces:**
- Consumes: `objectstore.Presigner`, `objectstore.Config`, `objectstore.New` (Task 2); `config.Config.S3*` (Task 1).
- Produces: `entity.ShadeAsset{ColorMapKey, AlphaMapKey, FinishMapKey string}` (JSON `-`); `controllers.NewMatchHandler(svc *service.MatchService, presigner objectstore.Presigner) *MatchHandler`; callback JSON fields `colorMapKey`, `alphaMapKey`, `finishMapKey`.

- [ ] **Step 1: Update the entity.** Replace the three URL fields of `ShadeAsset` with:

```go
	// Object keys in the tryon texture bucket (e.g. shades/<id>/color.png),
	// never URLs: GET /shade-assets/:id presigns a fresh link per request.
	// Nullable so AutoMigrate can add them to a populated table; the legacy
	// *_url columns are converted and dropped by cmd/shade-asset-keys.
	ColorMapKey  string    `json:"-" gorm:"type:varchar(512)"`
	AlphaMapKey  string    `json:"-" gorm:"type:varchar(512)"`
	FinishMapKey string    `json:"-" gorm:"type:varchar(512)"`
```

- [ ] **Step 2: Update the repo test.** In `TestShadeAsset_InMemory`, use `ColorMapKey: "shades/shade-1/color.png"`, `AlphaMapKey: "shades/shade-1/alpha.png"`, and compare `got.ColorMapKey != asset.ColorMapKey`.

- [ ] **Step 3: Write the failing handler tests.** In `match_handler_test.go`:

Add imports `context`, `errors`, and `github.com/experience-gateway/apps/core-engine/internal/match/adapters/objectstore`. Replace `newTestMatchHandler` and add helpers:

```go
type fakePresigner struct{ err error }

func (f fakePresigner) Presign(_ context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "https://signed.example/" + key + "?sig=1", nil
}

func newTestMatchHandlerWith(p objectstore.Presigner) *MatchHandler {
	repo := repository.NewMatchRepo(nil)
	return NewMatchHandler(service.NewMatchService(repo, ""), p)
}

func newTestMatchHandler() *MatchHandler { return newTestMatchHandlerWith(fakePresigner{}) }

func getShadeAsset(t *testing.T, h *MatchHandler, id string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/matching/shade-assets/"+id, nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := h.GetShadeAsset(c); err != nil {
		t.Fatalf("GetShadeAsset error: %v", err)
	}
	return rec
}

func saveAsset(t *testing.T, h *MatchHandler, a entity.ShadeAsset) string {
	t.Helper()
	saved, err := h.svc.SaveShadeAsset(context.Background(), a)
	if err != nil {
		t.Fatalf("SaveShadeAsset: %v", err)
	}
	return saved.ID
}
```

In `TestExtractionCallback_UpdatesStatusAndAsset`, send keys instead of URLs:

```go
	cbBody, _ := json.Marshal(map[string]interface{}{
		"status":       "ready",
		"colorMapKey":  "shades/s1/color.png",
		"alphaMapKey":  "shades/s1/alpha.png",
		"finishMapKey": "shades/s1/finish.png",
	})
```

and replace its final assertion block (from `var assetResp` on) with:

```go
	var assetResp struct {
		Success bool              `json:"success"`
		Asset   map[string]string `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &assetResp); err != nil {
		t.Fatalf("decode: %v body=%s", err, getRec.Body.String())
	}
	if !assetResp.Success || assetResp.Asset["colorMapUrl"] != "https://signed.example/shades/s1/color.png?sig=1" ||
		assetResp.Asset["finishMapUrl"] != "https://signed.example/shades/s1/finish.png?sig=1" {
		t.Fatalf("GetShadeAsset response = %s, want signed URLs", getRec.Body.String())
	}
	if _, leaked := assetResp.Asset["colorMapKey"]; leaked {
		t.Fatalf("response exposes stored keys: %s", getRec.Body.String())
	}
```

(`createdAt` is a string in JSON, so `map[string]string` decodes the whole object.)

At line 139 change `h := NewMatchHandler(svc)` to `h := NewMatchHandler(svc, fakePresigner{})`. In `envelope_test.go:22` pass `nil` as the second argument.

Append new tests:

```go
func TestExtractionCallback_ReadyWithoutKeysIs400(t *testing.T) {
	h := newTestMatchHandler()
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"status": "ready", "colorMapUrl": "https://old.example/c.png"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("any")
	if err := h.ExtractionCallback(c); err != nil {
		t.Fatalf("ExtractionCallback error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_UnconfiguredStorageIs503(t *testing.T) {
	h := newTestMatchHandlerWith(nil)
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("object storage not configured")) {
		t.Fatalf("status=%d body=%s, want 503 object storage not configured", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_NotFoundIs404(t *testing.T) {
	rec := getShadeAsset(t, newTestMatchHandler(), "missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetShadeAsset_RowWithoutKeysIs409(t *testing.T) {
	h := newTestMatchHandler()
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("shade-asset-keys")) {
		t.Fatalf("status=%d body=%s, want 409 naming the migration", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_SignFailureIs500WithoutEcho(t *testing.T) {
	h := newTestMatchHandlerWith(fakePresigner{err: errors.New("secret-internal-detail")})
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusInternalServerError || bytes.Contains(rec.Body.Bytes(), []byte("secret-internal-detail")) {
		t.Fatalf("status=%d body=%s, want 500 without the internal error", rec.Code, rec.Body.String())
	}
}

func TestGetShadeAsset_NoFinishKeyOmitsFinishMapUrl(t *testing.T) {
	h := newTestMatchHandler()
	id := saveAsset(t, h, entity.ShadeAsset{ShadeID: "s", ColorMapKey: "c", AlphaMapKey: "a"})
	rec := getShadeAsset(t, h, id)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("finishMapUrl")) {
		t.Fatalf("status=%d body=%s, want 200 without finishMapUrl", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 4: Run to verify failure.** `go test ./apps/core-engine/internal/match/...` → FAIL to compile (`NewMatchHandler` takes one argument; `ColorMapURL` undefined).

- [ ] **Step 5: Implement the handler.** In `match_handler.go`:

Imports: add `"log/slog"`, `"time"`, and `"github.com/experience-gateway/apps/core-engine/internal/match/adapters/objectstore"`. Keep `"log"` only if still used elsewhere in the file.

```go
type MatchHandler struct {
	svc       *service.MatchService
	presigner objectstore.Presigner
}

// NewMatchHandler takes a nil presigner when object storage is not
// configured; shade asset reads then answer 503.
func NewMatchHandler(svc *service.MatchService, presigner objectstore.Presigner) *MatchHandler {
	return &MatchHandler{svc: svc, presigner: presigner}
}
```

Replace `GetShadeAsset` (keep its existing doc comment's first line, update the rest):

```go
// shadeAssetResponse is the web contract for a shade asset: URLs, freshly
// presigned per request. The stored keys never leave the service.
type shadeAssetResponse struct {
	ID           string    `json:"id"`
	ShadeID      string    `json:"shadeId"`
	ColorMapURL  string    `json:"colorMapUrl"`
	AlphaMapURL  string    `json:"alphaMapUrl"`
	FinishMapURL string    `json:"finishMapUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (h *MatchHandler) GetShadeAsset(c echo.Context) error {
	if h.presigner == nil {
		return goplatform.JSONError(c, http.StatusServiceUnavailable, "object storage not configured")
	}
	ctx := c.Request().Context()
	asset, err := h.svc.GetShadeAssetByID(ctx, c.Param("id"))
	if err != nil {
		return goplatform.JSONError(c, http.StatusNotFound, "shade asset not found")
	}
	if asset.ColorMapKey == "" || asset.AlphaMapKey == "" {
		return goplatform.JSONError(c, http.StatusConflict, "shade asset has no storage keys; run the shade-asset-keys migration")
	}
	resp := shadeAssetResponse{ID: asset.ID, ShadeID: asset.ShadeID, CreatedAt: asset.CreatedAt}
	for _, m := range []struct {
		key string
		dst *string
	}{
		{asset.ColorMapKey, &resp.ColorMapURL},
		{asset.AlphaMapKey, &resp.AlphaMapURL},
		{asset.FinishMapKey, &resp.FinishMapURL},
	} {
		if m.key == "" {
			continue
		}
		u, err := h.presigner.Presign(ctx, m.key)
		if err != nil {
			slog.ErrorContext(ctx, "presign shade asset failed", "asset", asset.ID, "err", err)
			return goplatform.JSONError(c, http.StatusInternalServerError, "could not sign shade asset links")
		}
		*m.dst = u
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "asset": resp})
}
```

In `ExtractionCallback`, replace the payload struct's URL fields and add validation right after the status check:

```go
		ColorMapKey  string `json:"colorMapKey,omitempty"`
		AlphaMapKey  string `json:"alphaMapKey,omitempty"`
		FinishMapKey string `json:"finishMapKey,omitempty"`
```

```go
	if payload.Status == "ready" && (payload.ColorMapKey == "" || payload.AlphaMapKey == "") {
		return goplatform.JSONError(c, http.StatusBadRequest, "status 'ready' requires colorMapKey and alphaMapKey")
	}
```

and save `ColorMapKey: payload.ColorMapKey, AlphaMapKey: payload.AlphaMapKey, FinishMapKey: payload.FinishMapKey`.

- [ ] **Step 6: Wire `main.go`.** Add import `matchObjectStore "github.com/experience-gateway/apps/core-engine/internal/match/adapters/objectstore"` and replace the `matchHandler :=` line:

```go
	presigner, err := matchObjectStore.New(matchObjectStore.Config{
		Endpoint:        cfg.S3Endpoint,
		Bucket:          cfg.S3Bucket,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		Region:          cfg.S3Region,
		TTL:             cfg.S3PresignTTL,
	})
	if err != nil {
		return fmt.Errorf("object storage: %w", err)
	}
	if presigner == nil {
		deps.Log.Warn("object storage not configured; shade asset links unavailable (503)")
	}
	matchHandler := matchControllers.NewMatchHandler(mService, presigner)
```

- [ ] **Step 7: Run tests.** `go test ./apps/core-engine/...` → PASS. `npm run lint` → clean.

- [ ] **Step 8: Commit.**

```bash
node scripts/go-tools.mjs fmt
git add apps/core-engine/internal/match apps/core-engine/cmd/server/main.go
git commit -m "feat(core-engine): store shade texture keys and presign them on read"
```

---

### Task 4: ai-worker sends keys; public URLs removed

**Files:**
- Modify: `apps/ai-worker/ai_worker/tryon/services/storage.py`
- Modify: `apps/ai-worker/ai_worker/tryon/routers/extraction.py:55-70`
- Modify: `apps/ai-worker/ai_worker/core/storage.py` (delete `public_url`)
- Modify: `apps/ai-worker/ai_worker/core/settings.py` (delete `s3_public_url`)
- Test: `apps/ai-worker/tests/core/test_storage.py`, `apps/ai-worker/tests/core/test_settings.py`, `apps/ai-worker/tests/tryon/test_extraction_endpoint.py`

**Interfaces:**
- Consumes: callback field names from Task 3: `colorMapKey`, `alphaMapKey`, `finishMapKey`.
- Produces: `ai_worker.tryon.services.storage.upload_bytes(key, data, content_type="image/png") -> str` returning `key`.

- [ ] **Step 1: Write the failing tests.**

`tests/core/test_storage.py`: remove `monkeypatch.delenv("S3_PUBLIC_URL", raising=False)` from the fixture; delete `test_public_url_matches_tryon_engine_construction`, `test_public_url_prefers_s3_public_url`, `test_tryon_upload_returns_the_public_url`; replace `test_tryon_upload_returns_public_url` with:

```python
async def test_tryon_upload_returns_the_object_key(s3):
    # core-engine stores the key and presigns a link per read; the bucket is private.
    assert await tryon_storage.upload_bytes("shades/9/alpha.png", b"png") == "shades/9/alpha.png"
    assert s3.calls[0]["ContentType"] == "image/png"


def test_core_storage_has_no_public_url():
    assert not hasattr(storage, "public_url")
```

`tests/core/test_settings.py`: remove `"S3_PUBLIC_URL"` from `ENV` and delete `test_s3_public_url_is_trimmed_and_optional`.

`tests/tryon/test_extraction_endpoint.py`: append

```python
@pytest.mark.asyncio
async def test_ready_callback_carries_object_keys_not_urls():
    from ai_worker.tryon.routers import extraction as extraction_module

    fake_face = Image.new("RGB", (64, 64), color=(200, 180, 160))
    fake_mask = np.zeros((64, 64), dtype=bool)
    fake_mask[20:40, 20:40] = True
    # Exactly the requested #C41E3A, so the quality gate passes.
    fake_generated = Image.new("RGB", (64, 64), color=(0xC4, 0x1E, 0x3A))
    maps = {name: Image.new("RGB", (8, 8)) for name in ("color", "alpha", "finish")}

    req = ExtractionRequest(
        shade_id="shade-ok",
        reference_photo_url="https://example.com/face.jpg",
        hex_color="#C41E3A",
        region="lip",
    )
    captured_calls = []

    class _FakeResponse:
        content = b"fake-image-bytes"

        def raise_for_status(self):
            pass

    class _FakeAsyncClient:
        def __init__(self, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def get(self, url):
            return _FakeResponse()

        async def post(self, url, json):
            captured_calls.append(json)
            return _FakeResponse()

    upload = AsyncMock(side_effect=lambda key, data: key)
    with patch("ai_worker.tryon.routers.extraction.httpx.AsyncClient", _FakeAsyncClient), \
         patch("ai_worker.tryon.routers.extraction.Image.open", return_value=fake_face), \
         patch("ai_worker.tryon.routers.extraction.parse_face_regions", return_value={"lip": fake_mask}), \
         patch("ai_worker.tryon.routers.extraction.generate_shade_region", return_value=fake_generated), \
         patch("ai_worker.tryon.routers.extraction.extract_uv_texture", return_value=maps), \
         patch("ai_worker.tryon.routers.extraction.upload_bytes", upload):
        await extraction_module.run_extraction_job(req)

    assert captured_calls == [{
        "status": "ready",
        "colorMapKey": "shades/shade-ok/color.png",
        "alphaMapKey": "shades/shade-ok/alpha.png",
        "finishMapKey": "shades/shade-ok/finish.png",
    }]
```

- [ ] **Step 2: Run to verify failure.** `pytest apps/ai-worker/tests/core -q` → FAIL (`upload_bytes` returns a URL; `public_url` exists). `npm run test:tryon` (needs the `[tryon]` extra) → the new test FAILS on `colorMapUrl`. If the tryon extra is not installed, the tryon test is not collected: say so in the report, do not claim it passed.

- [ ] **Step 3: Implement.**

`ai_worker/tryon/services/storage.py`:

```python
from ai_worker.core import storage
from ai_worker.core.settings import settings


async def upload_bytes(key: str, data: bytes, content_type: str = "image/png") -> str:
    """Upload one texture map and return its object key. core-engine stores
    the key and presigns a short-lived link on every read, so the bucket
    stays private. The shared storage's local-disk fallback must never apply
    here: core-engine can only sign S3 keys. Require S3 up front."""
    settings.require_s3()
    await storage.upload_bytes(key, data, content_type=content_type)
    return key
```

`ai_worker/tryon/routers/extraction.py`: rename `urls` to `keys` in the upload loop (`keys[map_name] = await upload_bytes(key, buf.getvalue())`) and post

```python
                json={
                    "status": "ready",
                    "colorMapKey": keys["color"],
                    "alphaMapKey": keys["alpha"],
                    "finishMapKey": keys["finish"],
                },
```

`ai_worker/core/storage.py`: delete the `public_url` function entirely. `ai_worker/core/settings.py`: delete the `s3_public_url` property entirely.

Then `grep -rn "public_url\|S3_PUBLIC_URL" apps/ai-worker/ai_worker apps/ai-worker/tests` → only `vision_worker_public_url` remains.

- [ ] **Step 4: Run tests.** `npm run test:vision` → PASS. `npm run test:tryon` → PASS (or state it was not collected).

- [ ] **Step 5: Commit.**

```bash
git add apps/ai-worker/ai_worker/tryon apps/ai-worker/ai_worker/core/storage.py apps/ai-worker/ai_worker/core/settings.py apps/ai-worker/tests/core/test_storage.py apps/ai-worker/tests/core/test_settings.py apps/ai-worker/tests/tryon/test_extraction_endpoint.py
git commit -m "feat(ai-worker): tryon reports texture object keys, not public URLs"
```

---

### Task 5: `shade-asset-keys` migration command

**Files:**
- Create: `apps/core-engine/cmd/shade-asset-keys/keys.go`, `store.go`, `main.go`
- Test: `apps/core-engine/cmd/shade-asset-keys/keys_test.go`, `main_test.go`

**Interfaces:**
- Consumes: `config.Load`, `goplatform.OpenPostgres`, `goplatform.DBOptions`, `goplatform.First` (existing, same use as `cmd/facearch-profile/main.go`).
- Produces: CLI `shade-asset-keys [-prepare | -apply] [-public-url U] [-endpoint E] [-bucket B] [-region R]`. Exit 0 ok, 1 failure/unrecognised rows, 2 usage.

- [ ] **Step 1: Write the failing pure tests** (`keys_test.go`):

```go
package main

import "testing"

func TestLegacyPrefixes(t *testing.T) {
	got := legacyPrefixes(prefixSource{PublicURL: "https://pub.example.dev/", Endpoint: "https://acct.example-storage.com/", Bucket: "bkt", Region: "auto"})
	want := []string{"https://pub.example.dev/", "https://acct.example-storage.com/bkt/"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLegacyPrefixesWithoutEndpointUsesAWSForm(t *testing.T) {
	// ai-worker's old public_url built https://s3.<region>.amazonaws.com/<bucket>/<key>
	// when S3_ENDPOINT was unset.
	got := legacyPrefixes(prefixSource{Bucket: "bkt", Region: "ap-southeast-1"})
	if len(got) != 1 || got[0] != "https://s3.ap-southeast-1.amazonaws.com/bkt/" {
		t.Fatalf("got %v", got)
	}
}

func TestKeyFromURL(t *testing.T) {
	prefixes := []string{"https://pub.example.dev/", "https://acct.example-storage.com/bkt/"}
	for _, tc := range []struct {
		url, key string
		ok       bool
	}{
		{"https://pub.example.dev/shades/1/color.png", "shades/1/color.png", true},
		{"https://acct.example-storage.com/bkt/shades/1/alpha.png", "shades/1/alpha.png", true},
		{"https://elsewhere.example/shades/1/color.png", "", false},
		{"https://pub.example.dev/", "", false},
	} {
		key, ok := keyFromURL(tc.url, prefixes)
		if key != tc.key || ok != tc.ok {
			t.Errorf("keyFromURL(%q) = %q, %v; want %q, %v", tc.url, key, ok, tc.key, tc.ok)
		}
	}
}

func TestPlanRowsAllOrNothing(t *testing.T) {
	prefixes := []string{"https://pub.example.dev/"}
	finish := "https://pub.example.dev/shades/1/finish.png"
	rows := []legacyRow{
		{ID: "a1", ColorURL: "https://pub.example.dev/shades/1/color.png", AlphaURL: "https://pub.example.dev/shades/1/alpha.png", FinishURL: &finish},
		{ID: "a2", ColorURL: "https://other.example/c.png", AlphaURL: "https://pub.example.dev/shades/2/alpha.png"},
	}
	planned, bad := planRows(rows, prefixes)
	if len(planned) != 1 || planned[0].FinishKey != "shades/1/finish.png" || planned[0].ColorKey != "shades/1/color.png" {
		t.Fatalf("planned = %+v", planned)
	}
	if len(bad) != 1 || bad[0] != "a2: color_map_url https://other.example/c.png" {
		t.Fatalf("bad = %v", bad)
	}
}
```

- [ ] **Step 2: Run to verify failure.** `go test ./apps/core-engine/cmd/shade-asset-keys/...` → FAIL (undefined).

- [ ] **Step 3: Implement `keys.go`.**

```go
package main

import (
	"fmt"
	"strings"
)

// prefixSource holds where legacy rows' URLs could have pointed. Every
// value comes from env or flags; see main.go.
type prefixSource struct {
	PublicURL, Endpoint, Bucket, Region string
}

// legacyPrefixes lists the two URL forms ai-worker's removed public_url()
// ever wrote: {S3_PUBLIC_URL}/{key}, else {endpoint}/{bucket}/{key} with the
// endpoint defaulting to AWS's regional host.
func legacyPrefixes(s prefixSource) []string {
	var out []string
	if p := strings.TrimRight(s.PublicURL, "/"); p != "" {
		out = append(out, p+"/")
	}
	endpoint := strings.TrimRight(s.Endpoint, "/")
	if endpoint == "" && s.Region != "" {
		endpoint = "https://s3." + s.Region + ".amazonaws.com"
	}
	if endpoint != "" && s.Bucket != "" {
		out = append(out, endpoint+"/"+s.Bucket+"/")
	}
	return out
}

func keyFromURL(url string, prefixes []string) (string, bool) {
	for _, p := range prefixes {
		if key, ok := strings.CutPrefix(url, p); ok && key != "" {
			return key, true
		}
	}
	return "", false
}

type legacyRow struct {
	ID                 string
	ColorURL, AlphaURL string
	FinishURL          *string
}

type keyedRow struct {
	ID                            string
	ColorKey, AlphaKey, FinishKey string
}

// planRows maps every row or reports why it cannot. The caller writes
// nothing unless bad is empty.
func planRows(rows []legacyRow, prefixes []string) (planned []keyedRow, bad []string) {
	for _, r := range rows {
		k := keyedRow{ID: r.ID}
		ok := true
		for _, f := range []struct {
			col string
			url *string
			dst *string
		}{
			{"color_map_url", &r.ColorURL, &k.ColorKey},
			{"alpha_map_url", &r.AlphaURL, &k.AlphaKey},
			{"finish_map_url", r.FinishURL, &k.FinishKey},
		} {
			if f.url == nil || *f.url == "" {
				continue
			}
			key, found := keyFromURL(*f.url, prefixes)
			if !found {
				bad = append(bad, fmt.Sprintf("%s: %s %s", r.ID, f.col, *f.url))
				ok = false
				continue
			}
			*f.dst = key
		}
		if ok {
			planned = append(planned, k)
		}
	}
	return planned, bad
}
```

- [ ] **Step 4: Run.** `go test ./apps/core-engine/cmd/shade-asset-keys/...` → PASS.

- [ ] **Step 5: Write failing CLI tests** (`main_test.go`) against a fake store:

```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type fakeStore struct {
	hasURLCols bool
	rows       []legacyRow
	prepared   bool
	applied    []keyedRow
}

func (f *fakeStore) HasURLColumns(context.Context) (bool, error) { return f.hasURLCols, nil }
func (f *fakeStore) LegacyRows(context.Context) ([]legacyRow, error) { return f.rows, nil }
func (f *fakeStore) Prepare(context.Context) error                  { f.prepared = true; return nil }
func (f *fakeStore) Apply(_ context.Context, rows []keyedRow) error { f.applied = rows; return nil }

var testPrefixes = prefixSource{PublicURL: "https://pub.example.dev"}

func runWith(t *testing.T, s *fakeStore, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := run(context.Background(), args, &out, &out, s, testPrefixes)
	return code, out.String()
}

func goodRows() []legacyRow {
	return []legacyRow{{ID: "a1", ColorURL: "https://pub.example.dev/shades/1/color.png", AlphaURL: "https://pub.example.dev/shades/1/alpha.png"}}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	code, out := runWith(t, s)
	if code != 0 || s.applied != nil || s.prepared {
		t.Fatalf("code=%d applied=%v prepared=%v", code, s.applied, s.prepared)
	}
	if !strings.Contains(out, "shades/1/color.png") || !strings.Contains(out, "dry run") {
		t.Fatalf("output = %q", out)
	}
}

func TestApplyWritesPlannedRows(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	if code, out := runWith(t, s, "-apply"); code != 0 || len(s.applied) != 1 {
		t.Fatalf("code=%d applied=%v out=%q", code, s.applied, out)
	}
}

func TestUnrecognisedRowAbortsWithoutWriting(t *testing.T) {
	rows := append(goodRows(), legacyRow{ID: "a2", ColorURL: "https://other.example/c.png", AlphaURL: "https://pub.example.dev/a.png"})
	s := &fakeStore{hasURLCols: true, rows: rows}
	code, out := runWith(t, s, "-apply")
	if code != 1 || s.applied != nil || !strings.Contains(out, "a2: color_map_url https://other.example/c.png") {
		t.Fatalf("code=%d applied=%v out=%q", code, s.applied, out)
	}
}

func TestAlreadyMigratedIsANoOp(t *testing.T) {
	s := &fakeStore{hasURLCols: false}
	code, out := runWith(t, s, "-apply")
	if code != 0 || s.applied != nil || !strings.Contains(out, "already") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestPrepareOnlyDropsConstraints(t *testing.T) {
	s := &fakeStore{hasURLCols: true, rows: goodRows()}
	if code, _ := runWith(t, s, "-prepare"); code != 0 || !s.prepared || s.applied != nil {
		t.Fatalf("code=%d prepared=%v applied=%v", code, s.prepared, s.applied)
	}
}

func TestPrepareAndApplyTogetherIsUsage(t *testing.T) {
	if code, _ := runWith(t, &fakeStore{hasURLCols: true}, "-prepare", "-apply"); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestNoPrefixesIsAnError(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"-apply"}, &out, &out, &fakeStore{hasURLCols: true, rows: goodRows()}, prefixSource{})
	if code != 1 || !strings.Contains(out.String(), "S3_PUBLIC_URL") {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}
```

- [ ] **Step 6: Run to verify failure.** `go test ./apps/core-engine/cmd/shade-asset-keys/...` → FAIL (undefined `run`).

- [ ] **Step 7: Implement `store.go`.**

```go
package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

const table = "match_shade_assets"

// store is the database side of the migration; tests replace it.
type store interface {
	HasURLColumns(ctx context.Context) (bool, error)
	LegacyRows(ctx context.Context) ([]legacyRow, error)
	// Prepare drops NOT NULL on the legacy URL columns so the new
	// core-engine, which no longer writes them, can insert.
	Prepare(ctx context.Context) error
	// Apply backfills the key columns and drops the URL columns in one
	// transaction.
	Apply(ctx context.Context, rows []keyedRow) error
}

type gormStore struct{ db *gorm.DB }

func (s gormStore) HasURLColumns(ctx context.Context) (bool, error) {
	return s.db.WithContext(ctx).Migrator().HasColumn(table, "color_map_url"), nil
}

func (s gormStore) LegacyRows(ctx context.Context) ([]legacyRow, error) {
	var raw []struct {
		ID           string
		ColorMapURL  *string `gorm:"column:color_map_url"`
		AlphaMapURL  *string `gorm:"column:alpha_map_url"`
		FinishMapURL *string `gorm:"column:finish_map_url"`
	}
	err := s.db.WithContext(ctx).Table(table).
		Select("id, color_map_url, alpha_map_url, finish_map_url").
		Where("color_map_url IS NOT NULL OR alpha_map_url IS NOT NULL").
		Order("id").Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	rows := make([]legacyRow, 0, len(raw))
	for _, r := range raw {
		row := legacyRow{ID: r.ID, FinishURL: r.FinishMapURL}
		if r.ColorMapURL != nil {
			row.ColorURL = *r.ColorMapURL
		}
		if r.AlphaMapURL != nil {
			row.AlphaURL = *r.AlphaMapURL
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s gormStore) Prepare(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec(
		"ALTER TABLE " + table + " ALTER COLUMN color_map_url DROP NOT NULL, ALTER COLUMN alpha_map_url DROP NOT NULL").Error
}

func (s gormStore) Apply(ctx context.Context, rows []keyedRow) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			updates := map[string]interface{}{"color_map_key": r.ColorKey, "alpha_map_key": r.AlphaKey}
			if r.FinishKey != "" {
				updates["finish_map_key"] = r.FinishKey
			}
			if err := tx.Table(table).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("backfill %s: %w", r.ID, err)
			}
		}
		return tx.Exec("ALTER TABLE " + table +
			" DROP COLUMN color_map_url, DROP COLUMN alpha_map_url, DROP COLUMN finish_map_url").Error
	})
}
```

(`Apply` fails loudly if the key columns do not exist yet, i.e. the new core-engine was not deployed first; that is intended.)

- [ ] **Step 8: Implement `main.go`.**

```go
// Command shade-asset-keys converts match_shade_assets rows written with
// public texture URLs into object keys, then drops the URL columns.
//
//	shade-asset-keys            dry run: print the planned URL -> key mapping
//	shade-asset-keys -prepare   drop NOT NULL on the URL columns (run before deploying)
//	shade-asset-keys -apply     backfill keys and drop the URL columns, in one transaction
//
// Recognised URL forms come from S3_PUBLIC_URL and S3_ENDPOINT/S3_BUCKET
// (flags override). A row matching neither aborts the run before anything
// is written. Reads DATABASE_URL like core-engine; touches only
// match_shade_assets, which core-engine owns.
//
// Spec: docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/packages/goplatform"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	db, err := goplatform.OpenPostgres(ctx, cfg.DatabaseURL, goplatform.DBOptions{
		Logger:             slog.Default(),
		SlowQueryThreshold: cfg.DBSlowQueryThreshold,
		PingTimeout:        cfg.DependencyPingTimeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		os.Exit(1)
	}
	defaults := prefixSource{
		PublicURL: goplatform.First("S3_PUBLIC_URL"),
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		Region:    cfg.S3Region,
	}
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, gormStore{db: db}, defaults)
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, s store, defaults prefixSource) int {
	fs := flag.NewFlagSet("shade-asset-keys", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prepare := fs.Bool("prepare", false, "only drop NOT NULL on the legacy URL columns")
	apply := fs.Bool("apply", false, "write keys and drop the legacy URL columns")
	src := defaults
	fs.StringVar(&src.PublicURL, "public-url", defaults.PublicURL, "legacy public base URL (default $S3_PUBLIC_URL)")
	fs.StringVar(&src.Endpoint, "endpoint", defaults.Endpoint, "S3 endpoint (default $S3_ENDPOINT)")
	fs.StringVar(&src.Bucket, "bucket", defaults.Bucket, "bucket (default $S3_BUCKET)")
	fs.StringVar(&src.Region, "region", defaults.Region, "region (default $S3_REGION)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*prepare && *apply) {
		fmt.Fprintln(stderr, "usage: shade-asset-keys [-prepare | -apply] [-public-url U] [-endpoint E] [-bucket B] [-region R]")
		return 2
	}

	has, err := s.HasURLColumns(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "inspect %s: %v\n", table, err)
		return 1
	}
	if !has {
		fmt.Fprintf(stdout, "%s has no URL columns: migration already done\n", table)
		return 0
	}
	if *prepare {
		if err := s.Prepare(ctx); err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "dropped NOT NULL on color_map_url and alpha_map_url")
		return 0
	}

	prefixes := legacyPrefixes(src)
	if len(prefixes) == 0 {
		fmt.Fprintln(stderr, "no legacy URL form known: set S3_PUBLIC_URL and/or S3_ENDPOINT+S3_BUCKET (or the flags)")
		return 1
	}
	rows, err := s.LegacyRows(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "read rows: %v\n", err)
		return 1
	}
	planned, bad := planRows(rows, prefixes)
	for _, r := range planned {
		fmt.Fprintf(stdout, "%s: color=%s alpha=%s finish=%s\n", r.ID, r.ColorKey, r.AlphaKey, r.FinishKey)
	}
	if len(bad) > 0 {
		fmt.Fprintf(stderr, "%d value(s) match no known URL form; nothing written:\n", len(bad))
		for _, b := range bad {
			fmt.Fprintln(stderr, "  "+b)
		}
		return 1
	}
	if !*apply {
		fmt.Fprintf(stdout, "dry run: %d row(s) would be converted; re-run with -apply\n", len(planned))
		return 0
	}
	if err := s.Apply(ctx, planned); err != nil {
		fmt.Fprintf(stderr, "apply: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "converted %d row(s) and dropped the URL columns\n", len(planned))
	return 0
}
```

- [ ] **Step 9: Run.** `go test ./apps/core-engine/cmd/shade-asset-keys/...` → PASS. `npm run lint` and `npm run build:check` → clean.

- [ ] **Step 10: Commit.**

```bash
node scripts/go-tools.mjs fmt
git add apps/core-engine/cmd/shade-asset-keys
git commit -m "feat(core-engine): shade-asset-keys command migrates URL rows to keys"
```

---

### Task 6: Docs

**Files:**
- Create: `docs/OBJECT-STORAGE.md`
- Modify: `docs/SHARED-DATABASE.md` (append a section)
- Modify: `docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md` (bucket-setup doc location)

- [ ] **Step 1: Write `docs/OBJECT-STORAGE.md`** with these sections, in prose, no credentials:
  1. **What lives in the bucket.** Tryon shade textures under `shades/<shadeId>/{color,alpha,finish}.png`; vision model files under `models/`. The bucket is private.
  2. **Configuration.** Table of `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_REGION` (default `auto`), read by ai-worker and core-engine; `S3_PRESIGN_TTL` (core-engine, default `15m`, policy). `S3_PUBLIC_URL` is read only by `shade-asset-keys`.
  3. **How the web app gets textures.** Tryon uploads and reports keys → core-engine stores keys → `GET /api/matching/shade-assets/:id` returns links presigned for `S3_PRESIGN_TTL`. Unconfigured storage = 503; unmigrated row = 409.
  4. **Bucket CORS (required).** The web app loads textures with `crossOrigin="anonymous"`, so the bucket needs a CORS rule: allowed origins = the web app's origins, methods `GET` and `HEAD`. On R2: bucket → Settings → CORS policy.
  5. **Migrating from public URLs.** In order: `go run ./apps/core-engine/cmd/shade-asset-keys -prepare`; deploy ai-worker and core-engine together; `go run ./apps/core-engine/cmd/shade-asset-keys` (dry run, check output); `... -apply`; disable the bucket's `r2.dev` public URL (R2: bucket → Settings → Public access).
  6. **Why not public URLs.** `*.r2.dev` is DNS-blocked in Indonesia (Internet Positif); presigned links use the S3 API host, which is not.

- [ ] **Step 2: Append to `docs/SHARED-DATABASE.md`:**

```markdown
## Column drops outside AutoMigrate

`AutoMigrate` never drops columns, so a drop is a deliberate, recorded step.

- **`match_shade_assets.color_map_url`, `alpha_map_url`, `finish_map_url`**
  (core-engine). Replaced by `*_map_key`. Dropped by
  `apps/core-engine/cmd/shade-asset-keys -apply` after converting each URL
  to its object key; see `docs/OBJECT-STORAGE.md`. No other repository
  reads this table.
```

- [ ] **Step 3: Fix the spec.** In the spec's "Bucket setup" heading, replace "documented in the storage section of `apps/ai-worker/README.md`" with "documented in `docs/OBJECT-STORAGE.md`" (there is no ai-worker README).

- [ ] **Step 4: Commit.**

```bash
git add docs/OBJECT-STORAGE.md docs/SHARED-DATABASE.md docs/superpowers/specs/2026-09-29-tryon-presigned-textures-design.md
git commit -m "docs: object storage, presigned shade textures, column drop record"
```

---

### Task 7: Verify, merge, end to end

- [ ] **Step 1: Full check in the worktree.** `npm run check` → PASS. `npm run test:tryon` → PASS or report not collected. Paste the summaries.

- [ ] **Step 2: Ask the user before touching the main checkout.** It has uncommitted edits to `apps/ai-worker/ai_worker/core/{settings.py,storage.py}` and `apps/ai-worker/tests/core/{test_settings.py,test_storage.py}` (the `S3_PUBLIC_URL` work), which this branch supersedes and which would block the merge. Show `git -C <main> diff --stat` for those four files and ask whether to discard them. Do not discard without a yes. Needs the `git` lane.

- [ ] **Step 3: Merge (with the user's yes).** In the main checkout: `git merge --no-ff feat/tryon-presigned-textures`. Never push.

- [ ] **Step 4: End to end against Neon + R2** (`.env` in the main checkout; claim the `runtime` lane and check `docs/PORTS.md` before starting core-engine on its port; never print `.env` values):
  1. Before starting the new core-engine, `go run ./apps/core-engine/cmd/shade-asset-keys -prepare`, then the dry run. Report its output.
  2. Start core-engine with `S3_PRESIGN_TTL=60s` (`npm run dev:core`); its `AutoMigrate` adds the `*_map_key` columns. Only now can `-apply` work: run it only if the user confirms (it drops columns in the shared Neon database). Without it, legacy rows answer 409, which is expected.
  3. Python probe from `apps/ai-worker`: upload a small PNG to `shades/e2e-probe-<ts>/color.png` and `alpha.png` via `ai_worker.tryon.services.storage.upload_bytes`; create a shade via `POST /api/matching/shades`; post `{"status":"ready","colorMapKey":...,"alphaMapKey":...}` to its extraction-callback.
  4. `GET /api/matching/shade-assets/<assetId>` → 200 with `colorMapUrl` on `*.r2.cloudflarestorage.com`; fetch it → 200 and the uploaded bytes.
  5. Wait past 60 s; fetch again → 403.
  6. Clean up: delete the probe objects (`ai_worker.core.storage.delete_object`), the shade (`DELETE /api/matching/shades`) and the asset row. Stop core-engine; release `runtime`.

- [ ] **Step 5: Release lanes.** Delete this session's row from `docs/LANES.md`. Remove the worktree only if the user agrees.

- [ ] **Step 6: Report** to the user: results with evidence, plus their operator steps: bucket CORS rule, disabling `r2.dev` after `-apply`, rotating the R2 token that was shared in chat.
