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
