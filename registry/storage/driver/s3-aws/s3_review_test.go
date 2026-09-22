package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/sirupsen/logrus"
)

func TestSDKLoggerRouting(t *testing.T) {
	isolateAWSConfig(t)
	for _, mode := range []string{"off", "debug"} {
		for _, status := range []int{http.StatusOK, http.StatusPartialContent} {
			t.Run(fmt.Sprintf("%s/%d", mode, status), func(t *testing.T) {
				var output bytes.Buffer
				logger := logrus.New()
				logger.SetOutput(&output)
				logger.SetLevel(logrus.DebugLevel)
				logger.SetFormatter(&logrus.JSONFormatter{})
				ctx := dcontext.WithLogger(t.Context(), logger.WithField("request-id", "test-request"))
				params := sdkTestParameters("https://storage.example")
				params["loglevel"] = mode
				d := driverFromParams(t, params)
				options := d.S3.Options()
				options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("payload")), Request: r}, nil
				})
				d.S3 = awss3.New(options)
				if _, err := d.GetContent(ctx, "/key"); err != nil {
					t.Fatal(err)
				}
				if mode == "off" {
					if output.Len() != 0 {
						t.Fatalf("logging disabled but got %s", output.String())
					}
					return
				}
				if !strings.Contains(output.String(), `"request-id":"test-request"`) || !strings.Contains(output.String(), `"storage.driver":"s3aws"`) {
					t.Fatalf("SDK logs bypassed contextual registry logger: %s", output.String())
				}
				warned := strings.Contains(output.String(), "Response has no supported checksum")
				if warned != (status == http.StatusOK) {
					t.Fatalf("checksum warning = %v for status %d", warned, status)
				}
			})
		}
	}
}

func TestLoggingOffRetainsChecksumValidation(t *testing.T) {
	isolateAWSConfig(t)
	d := driverFromParams(t, sdkTestParameters("https://storage.example"))
	options := d.S3.Options()
	options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"X-Amz-Checksum-Crc32": {"AAAAAA=="}},
			Body: io.NopCloser(strings.NewReader("payload")), Request: r,
		}, nil
	})
	d.S3 = awss3.New(options)
	if _, err := d.GetContent(t.Context(), "/key"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("bad checksum was not rejected with logging off: %v", err)
	}
}

func TestRegistryUserAgentUnexpectedRequest(t *testing.T) {
	stack := middleware.NewStack("GetObject", smithyhttp.NewStackRequest)
	if err := addRegistryUserAgent("registry-test")(stack); err != nil {
		t.Fatal(err)
	}
	m, ok := stack.Build.Get("RegistryUserAgent")
	if !ok {
		t.Fatal("middleware was not installed")
	}
	for _, request := range []any{nil, struct{}{}, (*smithyhttp.Request)(nil)} {
		_, _, err := m.HandleBuild(t.Context(), middleware.BuildInput{Request: request}, middleware.BuildHandlerFunc(func(context.Context, middleware.BuildInput) (middleware.BuildOutput, middleware.Metadata, error) {
			t.Error("invalid request was forwarded")
			return middleware.BuildOutput{}, middleware.Metadata{}, nil
		}))
		if err == nil || !strings.Contains(err.Error(), "unexpected request type") {
			t.Errorf("expected request-type error, got %v", err)
		}
	}
}

func TestRegistryUserAgentPresign(t *testing.T) {
	isolateAWSConfig(t)
	params := sdkTestParameters("https://storage.example")
	params["useragent"] = "registry-test"
	d := driverFromParams(t, params)
	options := d.S3.Options()
	var userAgents []string
	options.APIOptions = append(options.APIOptions, func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("InspectUserAgent", func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
			userAgents = append(userAgents, in.Request.(*smithyhttp.Request).Header.Get("User-Agent"))
			return next.HandleBuild(ctx, in)
		}), middleware.After)
	})
	options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("payload")), Request: r}, nil
	})
	d.S3 = awss3.New(options)
	d.presignClient = awss3.NewPresignClient(d.S3)
	if _, err := d.GetContent(t.Context(), "/key"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if _, err := d.RedirectURL(httptest.NewRequest(method, "https://registry.example", nil), "/key"); err != nil {
			t.Fatal(err)
		}
	}
	if len(userAgents) != 3 {
		t.Fatalf("observed %d requests, want a read and two presigns", len(userAgents))
	}
	if !strings.HasPrefix(userAgents[0], "aws-sdk-go-v2/") || !strings.HasSuffix(userAgents[0], " registry-test") {
		t.Errorf("backend User-Agent lost the SDK prefix or registry suffix: %q", userAgents[0])
	}
	for _, value := range userAgents[1:] {
		if value != "registry-test" {
			t.Errorf("presign User-Agent = %q, want registry-test without leading whitespace", value)
		}
	}
}

func TestWalkTraceParameters(t *testing.T) {
	isolateAWSConfig(t)
	var output bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&output)
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})
	ctx := dcontext.WithLogger(t.Context(), logger.WithField("request-id", "walk-test"))
	params := sdkTestParameters("https://storage.example")
	params["rootdirectory"] = "root"
	d := driverFromParams(t, params)
	options := d.S3.Options()
	options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<ListBucketResult/>")), Request: r}, nil
	})
	d.S3 = awss3.New(options)
	if err := d.Walk(ctx, "/dir", func(storagedriver.FileInfo) error { return nil }, storagedriver.WithStartAfterHint("/dir/previous")); err != nil {
		t.Fatal(err)
	}
	var entry struct {
		Message   string `json:"msg"`
		RequestID string `json:"request-id"`
	}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode walk trace: %v; output=%s", err, output.String())
	}
	want := `s3aws.ListObjectsV2(bucket="test-bucket", prefix="root/dir/", maxKeys=1000, startAfter="root/dir/previous")`
	if entry.Message != want || entry.RequestID != "walk-test" {
		t.Fatalf("unexpected walk trace: %+v", entry)
	}
}

func TestSDKV2ListPagination(t *testing.T) {
	isolateAWSConfig(t)
	for _, root := range []string{"", "root"} {
		for _, failPage := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/fail%d", root, failPage), func(t *testing.T) {
				params := sdkTestParameters("https://storage.example")
				params["rootdirectory"] = root
				d := driverFromParams(t, params)
				prefix := d.s3Path("/dir/")
				options := d.S3.Options()
				pages := 0
				options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
					pages++
					q := r.URL.Query()
					if q.Get("prefix") != prefix || q.Get("delimiter") != "/" || q.Get("max-keys") != "1000" {
						t.Errorf("pagination lost request parameters: %s", r.URL.RawQuery)
					}
					wantToken := ""
					if pages == 2 {
						wantToken = "next"
					}
					if q.Get("continuation-token") != wantToken || pages > 2 {
						t.Errorf("unexpected page %d token %q", pages, q.Get("continuation-token"))
					}
					status := http.StatusOK
					body := fmt.Sprintf("<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken><Contents><Key>%sa</Key></Contents><CommonPrefixes><Prefix>%ssub/</Prefix></CommonPrefixes></ListBucketResult>", prefix, prefix)
					if pages == 2 {
						body = fmt.Sprintf("<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>%sb</Key></Contents></ListBucketResult>", prefix)
					}
					if failPage == pages {
						status, body = http.StatusNotFound, "<Error><Code>NoSuchKey</Code></Error>"
					}
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})
				d.S3 = awss3.New(options)
				paths, err := d.List(t.Context(), "/dir")
				if failPage != 0 {
					var missing storagedriver.PathNotFoundError
					if !errors.As(err, &missing) || missing.Path != "/dir" || pages != failPage {
						t.Fatalf("page error lost path context: pages=%d, error=%v", pages, err)
					}
				} else if err != nil || pages != 2 || !slices.Equal(paths, []string{"/dir/a", "/dir/b", "/dir/sub"}) {
					t.Fatalf("list = %v, pages=%d, error=%v", paths, pages, err)
				}
			})
		}
	}
}

func TestCanceledRedirectKeepsSharedRefresh(t *testing.T) {
	isolateAWSConfig(t)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	started, release := make(chan struct{}), make(chan struct{}, 1)
	defer close(release)
	var calls atomic.Int32
	cache := aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(refreshCtx context.Context) (aws.Credentials, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return aws.Credentials{}, ctx.Err()
		}
		if refreshCtx.Err() != nil {
			return aws.Credentials{}, refreshCtx.Err()
		}
		return aws.Credentials{AccessKeyID: "access", SecretAccessKey: "secret"}, nil
	}))
	d := driverFromParams(t, sdkTestParameters("https://storage.example"))
	options := d.S3.Options()
	options.Credentials = cache
	options.ExpressCredentials = nil
	d.presignClient = awss3.NewPresignClient(awss3.New(options))
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := d.RedirectURL(httptest.NewRequest(http.MethodGet, "https://registry.example", nil).WithContext(requestCtx), "/key")
		result <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled request did not stop waiting: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Unblock the one shared refresh, without closing the cleanup channel twice.
	release <- struct{}{}
	if _, err := d.RedirectURL(httptest.NewRequest(http.MethodGet, "https://registry.example", nil).WithContext(ctx), "/key"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("credential refreshes = %d, want one shared refresh", calls.Load())
	}
}
