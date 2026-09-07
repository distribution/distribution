package s3

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestHTTP429Retries(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_NEW_RETRIES_2026", "false")
	for _, mode := range []string{"standard", "adaptive"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AWS_RETRY_MODE", mode)
			for _, tc := range []struct {
				name, body         string
				max, quota         any
				persistent, cancel bool
				attempts           int
				wantError          bool
			}{
				{name: "unrecognized code", body: "<Error><Code>TooManyRequests</Code></Error>", attempts: 2},
				{name: "empty response", attempts: 2},
				{name: "proxy response", body: "proxy rate limit exceeded", attempts: 2},
				{name: "no retries", max: 0, attempts: 1, wantError: true},
				{name: "attempt limit", max: 1, persistent: true, attempts: 2, wantError: true},
				{name: "quota exhausted", max: 3, quota: 1, attempts: 1, wantError: true},
				{name: "quota disabled", max: 3, quota: 0, persistent: true, attempts: 4, wantError: true},
				{name: "canceled", cancel: true, attempts: 1, wantError: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					params := sdkTestParameters("https://storage.example")
					params["maxretries"], params["retryquota"] = tc.max, tc.quota
					d := driverFromParams(t, params)
					options := d.S3.Options()
					options.Retryer = retry.AddWithMaxBackoffDelay(options.Retryer, time.Nanosecond)
					attempts := 0
					options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
						attempts++
						status, body := http.StatusTooManyRequests, tc.body
						if attempts > 1 && !tc.persistent {
							status, body = http.StatusOK, ""
						}
						if tc.cancel {
							cancel()
						}
						return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
					d.S3 = awss3.New(options)
					err := d.PutContent(ctx, "/key", []byte("payload"))
					if attempts != tc.attempts || (err != nil) != tc.wantError {
						t.Fatalf("attempts = %d, error = %v; want %d, error = %v", attempts, err, tc.attempts, tc.wantError)
					}
					if tc.cancel && !errors.Is(err, context.Canceled) {
						t.Fatalf("expected cancellation, got %v", err)
					}
				})
			}
		})
	}
}

type attemptTokenRetryer struct {
	aws.NopRetryer
	called, released bool
}

func (r *attemptTokenRetryer) GetAttemptToken(ctx context.Context) (func(error) error, error) {
	r.called = true
	return func(error) error { r.released = true; return nil }, ctx.Err()
}

func TestHTTP429RetryerPreservesAttemptTokens(t *testing.T) {
	underlying := &attemptTokenRetryer{}
	r := retryHTTP429{Retryer: underlying}
	release, err := r.GetAttemptToken(t.Context())
	if err != nil || !underlying.called {
		t.Fatalf("SDK attempt token was bypassed: %v", err)
	}
	if err := release(nil); err != nil || !underlying.released {
		t.Fatalf("SDK token release was bypassed: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.GetAttemptToken(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("SDK attempt-token error was lost: %v", err)
	}
	r = retryHTTP429{Retryer: aws.NopRetryer{}}
	if release, err := r.GetAttemptToken(t.Context()); err != nil || release == nil {
		t.Fatalf("Retryer v1 fallback failed: %v", err)
	}
}

func TestRegistryRetryConfiguration(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_CA_BUNDLE", "")
	for _, source := range []string{"environment", "shared config"} {
		t.Run(source, func(t *testing.T) {
			if source == "environment" {
				t.Setenv("AWS_MAX_ATTEMPTS", "9")
				t.Setenv("AWS_RETRY_MODE", "adaptive")
			} else {
				filename := filepath.Join(t.TempDir(), "config")
				if err := os.WriteFile(filename, []byte("[default]\nmax_attempts = 9\nretry_mode = adaptive\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_CONFIG_FILE", filename)
			}
			for _, tc := range []struct {
				name     string
				max      any
				quota    any
				attempts int
			}{
				{"omitted", nil, nil, 9},
				{"no retries", 0, nil, 1},
				{"configured retries", "3", nil, 4},
				{"quota only", nil, 0, 9},
				{"custom quota", nil, "10", 9},
				{"both", 3, 0, 4},
			} {
				t.Run(tc.name, func(t *testing.T) {
					params := sdkTestParameters("https://storage.example")
					params["maxretries"], params["retryquota"] = tc.max, tc.quota
					options := driverFromParams(t, params).S3.Options()
					if options.Retryer.MaxAttempts() != tc.attempts || options.RetryMode != aws.RetryModeAdaptive {
						t.Fatalf("retry configuration = (%v, %d), want (adaptive, %d)", options.RetryMode, options.Retryer.MaxAttempts(), tc.attempts)
					}
				})
			}
		})
	}
}

func TestRegistryRetryParameterValidation(t *testing.T) {
	for _, name := range []string{"maxretries", "retryquota"} {
		for _, value := range []any{-1, true, 1.5, "1.5", "3retries", "", "99999999999999999999999"} {
			params := sdkTestParameters("https://storage.example")
			params[name] = value
			if _, err := FromParameters(t.Context(), params); err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("%s=%v: expected parameter error, got %v", name, value, err)
			}
		}
	}
	params := sdkTestParameters("https://storage.example")
	params["maxretries"] = math.MaxInt
	if _, err := FromParameters(t.Context(), params); err == nil || !strings.Contains(err.Error(), "maxretries") {
		t.Errorf("overflowing attempts: expected maxretries error, got %v", err)
	}
	for _, params := range []DriverParameters{
		{MaxRetries: aws.Int(-1)},
		{MaxRetries: aws.Int(math.MaxInt)},
		{RetryQuota: aws.Int(-1)},
	} {
		if _, err := New(t.Context(), params); err == nil {
			t.Error("New accepted invalid retry configuration")
		}
	}
}

func TestRegistryRetryAttempts(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_CA_BUNDLE", "")
	t.Setenv("AWS_NEW_RETRIES_2026", "false")
	for _, tc := range []struct {
		name     string
		max      any
		quota    any
		attempts int
		exceeded bool
	}{
		{"defaults", nil, nil, 3, false},
		{"no retries", 0, nil, 1, false},
		{"four attempts", 3, nil, 4, false},
		{"quota exhausted", 3, 5, 2, true},
		{"quota disabled", 3, 0, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := sdkTestParameters("http://storage.example")
			params["maxretries"], params["retryquota"] = tc.max, tc.quota
			d := driverFromParams(t, params)
			options := d.S3.Options()
			// Exercise the actual request pipeline with only backoff shortened.
			options.Retryer = retry.AddWithMaxBackoffDelay(options.Retryer, time.Nanosecond)
			attempts := 0
			options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Content-Type": {"application/xml"}},
					Body:       io.NopCloser(strings.NewReader("<Error><Code>InternalError</Code></Error>")),
					Request:    r,
				}, nil
			})
			d.S3 = awss3.New(options)
			err := d.PutContent(t.Context(), "/key", []byte("payload"))
			if err == nil || attempts != tc.attempts {
				t.Fatalf("attempts = %d, want %d; error = %v", attempts, tc.attempts, err)
			}
			var exceeded ratelimit.QuotaExceededError
			if errors.As(err, &exceeded) != tc.exceeded {
				t.Fatalf("quota exceeded = %v, want %v: %v", errors.As(err, &exceeded), tc.exceeded, err)
			}
		})
	}
}

func TestRegistryRetryQuota(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_CA_BUNDLE", "")
	t.Setenv("AWS_NEW_RETRIES_2026", "false")
	for _, mode := range []string{"standard", "adaptive"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AWS_RETRY_MODE", mode)
			for _, tc := range []struct {
				name      string
				quota     any
				available int
			}{
				{"SDK default", nil, 100},
				{"custom capacity", 10, 2},
				{"disabled", 0, 1000},
			} {
				t.Run(tc.name, func(t *testing.T) {
					params := sdkTestParameters("https://storage.example")
					params["retryquota"] = tc.quota
					first := driverFromParams(t, params).S3.Options().Retryer
					second := driverFromParams(t, params).S3.Options().Retryer
					failure := errors.New("failed attempt")
					for range tc.available {
						// Do not release tokens: simulate consecutive failed attempts.
						if _, err := first.GetRetryToken(t.Context(), failure); err != nil {
							t.Fatal(err)
						}
					}
					_, err := first.GetRetryToken(t.Context(), failure)
					var exceeded ratelimit.QuotaExceededError
					if tc.name == "disabled" {
						if err != nil {
							t.Fatal(err)
						}
					} else if !errors.As(err, &exceeded) {
						t.Fatalf("expected exhausted quota, got %v", err)
					}
					if _, err := second.GetRetryToken(t.Context(), failure); err != nil {
						t.Fatalf("quota leaked across clients: %v", err)
					}
				})
			}
		})
	}
}
