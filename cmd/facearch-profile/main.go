// Command facearch-profile validates and stores face-architecture profiles.
//
//	facearch-profile validate <file.json>
//	facearch-profile upsert <file.json> --brand <id|*> --app <id|*> --code <code> [--title <title>]
//
// Both fetch the ai-worker catalogue (FACE_WORKER_URL, timeout
// FACE_MEASURE_TIMEOUT) and validate the document against it; validate
// never touches the database. upsert then stores the document as the next
// version of (brand, app, code), makes it active, and deactivates every
// other active profile in that brand+app scope. upsert reads DATABASE_URL
// like core-engine and migrates only its own table,
// face_architecture_profiles.
//
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md, "Profile CLI".
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/config"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/measureclient"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/facearch/profile"
	goplatform "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/platform"
)

// deps are the outside world; tests replace them.
type deps struct {
	catalogue func(ctx context.Context) (*measureclient.Catalogue, error)
	store     func(ctx context.Context) (profile.Store, func(), error)
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, liveDeps()))
}

const usage = `usage:
  facearch-profile validate <file.json>
  facearch-profile upsert <file.json> --brand <id|*> --app <id|*> --code <code> [--title <title>]
`

func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, file := args[0], args[1]
	switch cmd {
	case "validate":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if _, ok := load(ctx, file, stdout, stderr, d); !ok {
			return 1
		}
		return 0
	case "upsert":
		fs := flag.NewFlagSet("upsert", flag.ContinueOnError)
		fs.SetOutput(stderr)
		brand := fs.String("brand", "", "brand id, or * for any brand")
		app := fs.String("app", "", "application id, or * for any application")
		code := fs.String("code", "", "profile code")
		title := fs.String("title", "", "profile title (defaults to the code)")
		if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 || *brand == "" || *app == "" || *code == "" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		raw, ok := load(ctx, file, stdout, stderr, d)
		if !ok {
			return 1
		}
		store, closeStore, err := d.store(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "database: %v\n", err)
			return 1
		}
		defer closeStore()
		p, err := store.Upsert(ctx, profile.UpsertInput{BrandID: *brand, ApplicationID: *app, Code: *code, Title: *title, Document: raw})
		if err != nil {
			fmt.Fprintf(stderr, "upsert: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "stored %s version %d for brand %s app %s (id %s), now active\n", p.Code, p.Version, p.BrandID, p.ApplicationID, p.ID)
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// load reads, parses and validates file against the live catalogue.
func load(ctx context.Context, file string, stdout, stderr io.Writer, d deps) ([]byte, bool) {
	raw, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", file, err)
		return nil, false
	}
	doc, err := profile.Parse(raw)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", file, err)
		return nil, false
	}
	cat, err := d.catalogue(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "catalogue: %v\n", err)
		return nil, false
	}
	if err := profile.Validate(doc, cat); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", file, err)
		return nil, false
	}
	templates, rules := 0, 0
	if doc.Guidance != nil {
		templates, rules = len(doc.Guidance.Templates), len(doc.Guidance.Rules)
	}
	fmt.Fprintf(stdout, "%s: valid against %s (%d classifiers, %d traits, %d guidance templates, %d guidance rules, calibration %s)\n",
		file, cat.CatalogueVersion, len(doc.Classifiers), len(doc.Traits), templates, rules, doc.Calibration.Status)
	return raw, true
}

func liveDeps() deps {
	return deps{
		catalogue: func(ctx context.Context) (*measureclient.Catalogue, error) {
			goplatform.LoadDotEnv()
			url := goplatform.First("FACE_WORKER_URL")
			if url == "" {
				url = config.DefaultFaceWorkerURL
			}
			timeout, err := goplatform.Duration("FACE_MEASURE_TIMEOUT", config.DefaultFaceMeasureTimeout)
			if err != nil {
				return nil, err
			}
			return measureclient.New(url, timeout).Catalogue(ctx)
		},
		store: func(ctx context.Context) (profile.Store, func(), error) {
			cfg, err := config.Load()
			if err != nil {
				return nil, nil, err
			}
			db, err := goplatform.OpenPostgres(ctx, cfg.DatabaseURL, goplatform.DBOptions{
				Logger:             slog.Default(),
				SlowQueryThreshold: cfg.DBSlowQueryThreshold,
				PingTimeout:        cfg.DependencyPingTimeout,
			})
			if err != nil {
				return nil, nil, err
			}
			closeDB := func() {
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			}
			// Only core-engine's own table; the server migrates the rest.
			if err := db.AutoMigrate(&profile.FaceArchitectureProfile{}); err != nil {
				closeDB()
				return nil, nil, fmt.Errorf("migrate face_architecture_profiles: %w", err)
			}
			return profile.NewGormStore(db), closeDB, nil
		},
	}
}
