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
