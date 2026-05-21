package objectstore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config describes the raw object store used by RawSink.
type Config struct {
	Backend         string
	LocalRoot       string
	S3Bucket        string
	S3EndpointURL   string
	S3Region        string
	S3AccessKeyID   string
	S3SecretKey     string
	S3Prefix        string
	S3PathStyle     bool
	S3PublicBaseURL string
}

// NewConfigured returns the object store selected by OBJECT_STORE.
// OBJECT_STORE defaults to "local" for development. Use OBJECT_STORE=s3 for
// Cloudflare R2 or any S3-compatible provider.
func NewConfigured(ctx context.Context, localRoot string) (Store, error) {
	cfg := Config{
		Backend:         env("OBJECT_STORE", "local"),
		LocalRoot:       localRoot,
		S3Bucket:        os.Getenv("S3_BUCKET_RAW"),
		S3EndpointURL:   os.Getenv("S3_ENDPOINT_URL"),
		S3Region:        env("S3_REGION", "auto"),
		S3AccessKeyID:   os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretKey:     os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3Prefix:        env("S3_PREFIX", "raw"),
		S3PublicBaseURL: os.Getenv("S3_PUBLIC_BASE_URL"),
	}
	pathStyle, err := parseBoolEnv("S3_FORCE_PATH_STYLE", false)
	if err != nil {
		return nil, err
	}
	cfg.S3PathStyle = pathStyle
	return New(ctx, cfg)
}

// New constructs a Store from explicit config.
func New(ctx context.Context, cfg Config) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "local", "fs", "filesystem":
		return NewFS(cfg.LocalRoot)
	case "s3", "r2":
		return NewS3(ctx, S3Config{
			Bucket:        cfg.S3Bucket,
			EndpointURL:   cfg.S3EndpointURL,
			Region:        cfg.S3Region,
			AccessKeyID:   cfg.S3AccessKeyID,
			SecretKey:     cfg.S3SecretKey,
			Prefix:        cfg.S3Prefix,
			UsePathStyle:  cfg.S3PathStyle,
			PublicBaseURL: cfg.S3PublicBaseURL,
		})
	default:
		return nil, fmt.Errorf("objectstore: unsupported OBJECT_STORE %q", cfg.Backend)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseBoolEnv(key string, def bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return v, nil
}
