package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config configures an S3-compatible object store. It works with Cloudflare
// R2, AWS S3, Backblaze B2, MinIO, and similar providers.
type S3Config struct {
	Bucket        string
	EndpointURL   string
	Region        string
	AccessKeyID   string
	SecretKey     string
	Prefix        string
	UsePathStyle  bool
	PublicBaseURL string
}

// S3 is an S3-compatible Store.
type S3 struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("objectstore s3: S3_BUCKET_RAW is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto"
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretKey != "" {
		if cfg.AccessKeyID == "" || cfg.SecretKey == "" {
			return nil, errors.New("objectstore s3: S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY must be set together")
		}
		loadOpts = append(loadOpts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, "")))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.EndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.EndpointURL)
		}
	})

	return &S3{
		client: client,
		bucket: cfg.Bucket,
		prefix: cleanPrefix(cfg.Prefix),
	}, nil
}

func (s *S3) Put(ctx context.Context, system, hash, contentType string, body []byte) (string, error) {
	if hash == "" {
		return "", errors.New("objectstore: hash required")
	}
	key := s.key(system, hash+extFor(contentType))

	if exists, err := s.exists(ctx, key); err != nil {
		return "", err
	} else if exists {
		return key, nil
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(body),
			ContentType: aws.String(contentType),
		})
		if err == nil {
			return key, nil
		}

		lastErr = err
		if !isRetryablePut(err) {
			return "", fmt.Errorf("s3 put %s: %w", key, err)
		}

		// Content-addressed writes are idempotent. If another worker won the race
		// while R2 throttled this PutObject, treat the existing object as success.
		if exists, headErr := s.exists(ctx, key); headErr == nil && exists {
			return key, nil
		} else if headErr != nil && !isRetryablePut(headErr) {
			return "", headErr
		}

		if err := sleepBackoff(ctx, attempt); err != nil {
			return "", err
		}
	}

	if exists, err := s.exists(ctx, key); err == nil && exists {
		return key, nil
	} else if err != nil && !isRetryablePut(err) {
		return "", err
	}
	return "", fmt.Errorf("s3 put %s after retries: %w", key, lastErr)
}

func (s *S3) exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("s3 head %s: %w", key, err)
}

func (s *S3) Get(ctx context.Context, relPath string) ([]byte, error) {
	key := strings.TrimLeft(relPath, "/")
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read %s: %w", key, err)
	}
	return body, nil
}

func isNotFound(err error) bool {
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}
	return false
}

func isRetryablePut(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ServiceUnavailable", "SlowDown", "TooManyRequests", "Throttling", "ThrottlingException", "RequestTimeout", "RequestTimeoutException":
			return true
		}
		msg := strings.ToLower(apiErr.ErrorMessage())
		if strings.Contains(msg, "reduce your concurrent request rate") || strings.Contains(msg, "rate") || strings.Contains(msg, "thrott") || strings.Contains(msg, "temporar") {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") || strings.Contains(msg, "reduce your concurrent request rate") || strings.Contains(msg, "serviceunavailable") || strings.Contains(msg, "slowdown") || strings.Contains(msg, "thrott") || strings.Contains(msg, "temporar")
}

func sleepBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(250*(1<<attempt)) * time.Millisecond
	if delay > 4*time.Second {
		delay = 4 * time.Second
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (s *S3) key(system, filename string) string {
	parts := []string{s.prefix, strings.Trim(system, "/"), strings.Trim(filename, "/")}
	return path.Join(parts...)
}

func cleanPrefix(prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "." {
		return ""
	}
	return prefix
}
