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
