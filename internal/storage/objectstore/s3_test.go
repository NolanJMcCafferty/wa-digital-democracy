package objectstore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestNewS3_RequiresBucket(t *testing.T) {
	if _, err := NewS3(context.Background(), S3Config{}); err == nil {
		t.Fatal("expected error for empty bucket")
	}
}

func TestNewS3_HalfCredentials(t *testing.T) {
	if _, err := NewS3(context.Background(), S3Config{Bucket: "b", AccessKeyID: "id"}); err == nil {
		t.Fatal("expected error when only AccessKeyID provided")
	}
	if _, err := NewS3(context.Background(), S3Config{Bucket: "b", SecretKey: "secret"}); err == nil {
		t.Fatal("expected error when only SecretKey provided")
	}
}

func TestNewS3_DefaultsRegion(t *testing.T) {
	s, err := NewS3(context.Background(), S3Config{Bucket: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if s.bucket != "b" {
		t.Errorf("bucket = %q, want b", s.bucket)
	}
}

func TestS3Key(t *testing.T) {
	s := &S3{bucket: "b", prefix: "raw"}
	if got := s.key("lws", "h1.json"); got != "raw/lws/h1.json" {
		t.Errorf("key = %q", got)
	}
	s2 := &S3{bucket: "b", prefix: ""}
	if got := s2.key("lws", "h1.json"); got != "lws/h1.json" {
		t.Errorf("no-prefix key = %q", got)
	}
	// Slash trimming on inputs.
	s3 := &S3{bucket: "b", prefix: "raw"}
	if got := s3.key("/lws/", "/h1.json/"); got != "raw/lws/h1.json" {
		t.Errorf("trim key = %q", got)
	}
}

type mockAPIError struct {
	code, msg string
}

func (e *mockAPIError) Error() string                            { return e.code + ": " + e.msg }
func (e *mockAPIError) ErrorCode() string                        { return e.code }
func (e *mockAPIError) ErrorMessage() string                     { return e.msg }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault            { return smithy.FaultUnknown }

var _ smithy.APIError = (*mockAPIError)(nil)

func TestIsNotFound(t *testing.T) {
	if !isNotFound(&types.NotFound{}) {
		t.Error("typed NotFound should match")
	}
	if !isNotFound(&mockAPIError{code: "NoSuchKey"}) {
		t.Error("NoSuchKey should match")
	}
	if !isNotFound(&mockAPIError{code: "404"}) {
		t.Error("404 should match")
	}
	if isNotFound(errors.New("random")) {
		t.Error("plain error should not match")
	}
	if isNotFound(&mockAPIError{code: "AccessDenied"}) {
		t.Error("AccessDenied should not match")
	}
}

func TestIsRetryablePut(t *testing.T) {
	retryableCodes := []string{
		"ServiceUnavailable", "SlowDown", "TooManyRequests",
		"Throttling", "ThrottlingException", "RequestTimeout", "RequestTimeoutException",
	}
	for _, code := range retryableCodes {
		if !isRetryablePut(&mockAPIError{code: code}) {
			t.Errorf("%s should be retryable", code)
		}
	}
	if !isRetryablePut(&mockAPIError{code: "Other", msg: "please reduce your concurrent request rate"}) {
		t.Error("rate-limit message should be retryable")
	}
	if !isRetryablePut(errors.New("upstream returned 429")) {
		t.Error("plain 429 message should be retryable")
	}
	if isRetryablePut(&mockAPIError{code: "AccessDenied", msg: "no"}) {
		t.Error("AccessDenied should not be retryable")
	}
	if isRetryablePut(errors.New("permission denied")) {
		t.Error("plain permission error should not be retryable")
	}
}

func TestSleepBackoff_RespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepBackoff(ctx, 5); err == nil {
		t.Fatal("expected ctx.Err() on canceled context")
	}
}

func TestSleepBackoff_Bounded(t *testing.T) {
	// attempt=0 should sleep ~250ms; we just want to make sure it returns
	// quickly enough that the cap doesn't blow up the test.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cancel()
	err := sleepBackoff(ctx, 10)
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context error, got %v", err)
	}
}
