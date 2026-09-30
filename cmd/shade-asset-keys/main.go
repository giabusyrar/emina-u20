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
