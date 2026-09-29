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

// r2PseudoRegion is the region value Cloudflare R2 uses; no AWS regional
// host exists for it, so it never yields the AWS-form prefix.
const r2PseudoRegion = "auto"

// legacyPrefixes lists the two URL forms ai-worker's removed public_url()
// ever wrote: {S3_PUBLIC_URL}/{key}, else {endpoint}/{bucket}/{key} with the
// endpoint defaulting to AWS's regional host.
func legacyPrefixes(s prefixSource) []string {
	var out []string
	if p := strings.TrimRight(s.PublicURL, "/"); p != "" {
		out = append(out, p+"/")
	}
	endpoint := strings.TrimRight(s.Endpoint, "/")
	if endpoint == "" && s.Region != "" && s.Region != r2PseudoRegion {
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
