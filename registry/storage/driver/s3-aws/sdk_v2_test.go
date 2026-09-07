package s3

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

func sdkTestParameters(endpoint string) map[string]any {
	return map[string]any{
		"accesskey": "access", "secretkey": "secret",
		"bucket": "test-bucket", "region": "us-east-1",
		"regionendpoint": endpoint, "forcepathstyle": true,
		"secure": false,
	}
}

func TestSDKV2RetryConfiguration(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_RETRY_MODE", "adaptive")
	t.Setenv("AWS_MAX_ATTEMPTS", "9")
	params := sdkTestParameters("https://storage.example")
	options := driverFromParams(t, params).S3.Options()
	if options.RetryMode != aws.RetryModeAdaptive || options.Retryer.MaxAttempts() != 9 {
		t.Fatalf("retry configuration = (%v, %d)", options.RetryMode, options.Retryer.MaxAttempts())
	}
}

func TestSDKV2ChecksumConfiguration(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		name, policy string
		request      aws.RequestChecksumCalculation
		response     aws.ResponseChecksumValidation
	}{
		{"default", "", aws.RequestChecksumCalculationWhenRequired, aws.ResponseChecksumValidationWhenSupported},
		{"configured", "when_required", aws.RequestChecksumCalculationWhenRequired, aws.ResponseChecksumValidationWhenRequired},
		{"supported", "when_supported", aws.RequestChecksumCalculationWhenSupported, aws.ResponseChecksumValidationWhenSupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_REQUEST_CHECKSUM_CALCULATION", tc.policy)
			t.Setenv("AWS_RESPONSE_CHECKSUM_VALIDATION", tc.policy)
			options := driverFromParams(t, sdkTestParameters("https://storage.example")).S3.Options()
			if options.RequestChecksumCalculation != tc.request || options.ResponseChecksumValidation != tc.response {
				t.Fatalf("checksum configuration = (%v, %v), want (%v, %v)", options.RequestChecksumCalculation, options.ResponseChecksumValidation, tc.request, tc.response)
			}
		})
	}
}

func TestSDKV2PutRequest(t *testing.T) {
	isolateAWSConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.EscapedPath() != "/test-bucket/root/space%20and%2Bplus" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		for name, want := range map[string]string{
			"Content-Type":                 "application/octet-stream",
			"X-Amz-Acl":                    "bucket-owner-full-control",
			"X-Amz-Storage-Class":          "STANDARD_IA",
			"X-Amz-Server-Side-Encryption": "aws:kms",
			"X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id": "key-id",
			"X-Amz-Security-Token":                        "session-token",
		} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		if !strings.Contains(r.UserAgent(), "registry-test") || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("missing configured user agent or SigV4 authorization")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "payload" || r.ContentLength != 7 {
			t.Errorf("body = %q, length = %d, error = %v", body, r.ContentLength, err)
		}
		checksum := crc32.NewIEEE()
		_, _ = checksum.Write(body)
		if r.Header.Get("X-Amz-Checksum-Crc32") != base64.StdEncoding.EncodeToString(checksum.Sum(nil)) {
			t.Error("missing or incorrect SDK default upload checksum")
		}
		w.Header().Set("ETag", `"etag"`)
	}))
	defer server.Close()
	params := sdkTestParameters(strings.TrimPrefix(server.URL, "http://"))
	params["rootdirectory"] = "/root"
	params["sessiontoken"] = "session-token"
	params["useragent"] = "registry-test"
	params["objectacl"] = "bucket-owner-full-control"
	params["storageclass"] = "STANDARD_IA"
	params["encrypt"] = true
	params["keyid"] = "key-id"
	params["requestchecksumcalculation"] = "when_supported"
	d := driverFromParams(t, params)
	if err := d.PutContent(t.Context(), "/space and+plus", []byte("payload")); err != nil {
		t.Fatal(err)
	}
}

func TestSDKV2RetryDefaults(t *testing.T) {
	isolateAWSConfig(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "<Error><Code>SlowDown</Code></Error>")
	}))
	defer server.Close()
	d := driverFromParams(t, sdkTestParameters(server.URL))
	_, err := d.S3.PutObject(t.Context(), &s3.PutObjectInput{
		Bucket: aws.String(d.Bucket), Key: aws.String("key"),
	}, func(o *s3.Options) {
		// Keep the production retry count, without waiting for random backoff.
		o.Retryer = retry.AddWithMaxBackoffDelay(o.Retryer, time.Nanosecond)
	})
	if !hasErrorCode(err, "SlowDown") {
		t.Fatalf("expected wrapped S3 SlowDown, got %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want the SDK default of three attempts", attempts.Load())
	}
}

func TestSDKV2DeleteObjectsContentMD5(t *testing.T) {
	isolateAWSConfig(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !r.URL.Query().Has("delete") {
			t.Error("unexpected bulk deletion request")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), "key&amp;with&lt;xml&gt;") {
			t.Error("bulk deletion body was consumed or incorrectly serialized")
		}
		sum := md5.Sum(body)
		if r.Header.Get("Content-MD5") != base64.StdEncoding.EncodeToString(sum[:]) {
			t.Error("Content-MD5 does not cover the actual serialized request body")
		}
		if !strings.Contains(r.Header.Get("Authorization"), "content-md5;") {
			t.Error("bulk deletion checksum was not signed")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "<Error><Code>SlowDown</Code></Error>")
			return
		}
		_, _ = io.WriteString(w, "<DeleteResult/>")
	}))
	defer server.Close()
	params := sdkTestParameters(server.URL)
	d := driverFromParams(t, params)
	_, err := d.S3.DeleteObjects(t.Context(), &s3.DeleteObjectsInput{
		Bucket: aws.String(d.Bucket),
		Delete: &types.Delete{Objects: []types.ObjectIdentifier{{Key: aws.String("key&with<xml>")}}},
	}, func(o *s3.Options) {
		o.Retryer = retry.AddWithMaxBackoffDelay(o.Retryer, time.Nanosecond)
	})
	if err != nil || attempts.Load() != 2 {
		t.Fatalf("bulk deletion retry: %v, %d attempts", err, attempts.Load())
	}
}

func driverFromParams(t *testing.T, params map[string]any) *driver {
	t.Helper()
	d, err := FromParameters(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	return d.StorageDriver.(*driver)
}

func TestSDKV2Credentials(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "environment-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "environment-secret")
	t.Setenv("AWS_SESSION_TOKEN", "environment-token")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, explicit := range []bool{false, true} {
		t.Run(strconv.FormatBool(explicit), func(t *testing.T) {
			params := sdkTestParameters("https://storage.example")
			want := aws.Credentials{AccessKeyID: "environment-access", SecretAccessKey: "environment-secret", SessionToken: "environment-token"}
			if explicit {
				params["sessiontoken"] = "explicit-token"
				want = aws.Credentials{AccessKeyID: "access", SecretAccessKey: "secret", SessionToken: "explicit-token"}
			} else {
				delete(params, "accesskey")
				delete(params, "secretkey")
			}
			d := driverFromParams(t, params)
			got, err := d.S3.Options().Credentials.Retrieve(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if got.AccessKeyID != want.AccessKeyID || got.SecretAccessKey != want.SecretAccessKey || got.SessionToken != want.SessionToken {
				t.Fatal("driver selected the wrong credential provider")
			}
		})
	}
}

func TestSDKV2PresignEndpoints(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		name   string
		params map[string]any
		host   string
		path   string
		scheme string
	}{
		{"path style", map[string]any{"regionendpoint": "https://storage.example", "forcepathstyle": true}, "storage.example", "/test-bucket/root/key%20with%2Bplus", "https"},
		{"virtual host", map[string]any{"regionendpoint": "https://storage.example"}, "test-bucket.storage.example", "/root/key%20with%2Bplus", "https"},
		{"redirect", map[string]any{"regionendpoint": "https://internal.example", "forcepathstyle": true, "redirectendpoint": "https://download.example:8443"}, "download.example:8443", "/test-bucket/root/key%20with%2Bplus", "https"},
		{"virtual host redirect", map[string]any{"regionendpoint": "https://internal.example", "redirectendpoint": "https://download.example:8443"}, "test-bucket.download.example:8443", "/root/key%20with%2Bplus", "https"},
		{"backend base path redirect", map[string]any{"regionendpoint": "https://internal.example/base%20path", "redirectendpoint": "https://download.example"}, "test-bucket.download.example", "/base%20path/root/key%20with+plus", "https"},
		{"dualstack backend redirect", map[string]any{"usedualstack": true, "redirectendpoint": "https://download.example"}, "test-bucket.download.example", "/root/key%20with%2Bplus", "https"},
		{"FIPS backend redirect", map[string]any{"usefipsendpoint": true, "redirectendpoint": "https://download.example"}, "test-bucket.download.example", "/root/key%20with%2Bplus", "https"},
		{"accelerate backend redirect", map[string]any{"accelerate": true, "redirectendpoint": "https://download.example"}, "test-bucket.download.example", "/root/key%20with%2Bplus", "https"},
		{"IP redirect", map[string]any{"regionendpoint": "https://internal.example", "redirectendpoint": "https://127.0.0.1:8443"}, "127.0.0.1:8443", "/test-bucket/root/key%20with%2Bplus", "https"},
		{"IPv6 redirect", map[string]any{"regionendpoint": "https://internal.example", "redirectendpoint": "https://[::1]:8443"}, "[::1]:8443", "/test-bucket/root/key%20with%2Bplus", "https"},
		{"IP backend virtual redirect", map[string]any{"regionendpoint": "http://127.0.0.1", "redirectendpoint": "https://download.example"}, "test-bucket.download.example", "/root/key%20with%2Bplus", "https"},
		{"dotted bucket redirect", map[string]any{"bucket": "test.bucket", "regionendpoint": "http://internal.example", "redirectendpoint": "https://download.example"}, "download.example", "/test.bucket/root/key%20with%2Bplus", "https"},
		{"dualstack", map[string]any{"usedualstack": true}, "test-bucket.s3.dualstack.us-east-1.amazonaws.com", "/root/key%20with%2Bplus", "https"},
		{"FIPS", map[string]any{"usefipsendpoint": true}, "test-bucket.s3-fips.us-east-1.amazonaws.com", "/root/key%20with%2Bplus", "https"},
		{"accelerate", map[string]any{"accelerate": true}, "test-bucket.s3-accelerate.amazonaws.com", "/root/key%20with%2Bplus", "https"},
		{"HTTP", map[string]any{"secure": false}, "test-bucket.s3.us-east-1.amazonaws.com", "/root/key%20with%2Bplus", "http"},
		{"schemeless HTTPS", map[string]any{"regionendpoint": "storage.example"}, "test-bucket.storage.example", "/root/key%20with%2Bplus", "https"},
		{"schemeless HTTP", map[string]any{"regionendpoint": "storage.example", "secure": false}, "test-bucket.storage.example", "/root/key%20with%2Bplus", "http"},
		{"future region", map[string]any{"region": "us-future-1"}, "test-bucket.s3.us-future-1.amazonaws.com", "/root/key%20with%2Bplus", "https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{"accesskey": "access", "secretkey": "secret", "bucket": "test-bucket", "region": "us-east-1", "rootdirectory": "/root"}
			for key, value := range tc.params {
				params[key] = value
			}
			d := driverFromParams(t, params)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				r := httptest.NewRequest(method, "http://registry.example/v2/", nil)
				signed, err := d.RedirectURL(r, "/key with+plus")
				if err != nil {
					t.Fatal(err)
				}
				u, err := url.Parse(signed)
				if err != nil {
					t.Fatal(err)
				}
				if u.Host != tc.host || u.EscapedPath() != tc.path || u.Scheme != tc.scheme {
					t.Errorf("%s endpoint = %s://%s%s", method, u.Scheme, u.Host, u.EscapedPath())
				}
				// '+' and '%2B' are equivalent in URL paths; verify the object
				// key independently of which spelling the SDK uses for a base path.
				if !strings.HasSuffix(u.Path, "/root/key with+plus") {
					t.Errorf("decoded object key changed: %q", u.Path)
				}
				q := u.Query()
				if q.Get("X-Amz-Expires") != "1200" || q.Get("X-Amz-SignedHeaders") != "host" || q.Get("X-Amz-Signature") == "" {
					t.Error("presign did not produce a self-contained 20-minute signed URL")
				}
			}
		})
	}
}

func TestSDKV2WrappedMissingKey(t *testing.T) {
	isolateAWSConfig(t)
	err := &smithy.OperationError{ServiceID: "S3", OperationName: "GetObject", Err: &smithy.GenericAPIError{Code: "NoSuchKey"}}
	var missing storagedriver.PathNotFoundError
	if !errors.As(parseError("/missing", err), &missing) || missing.Path != "/missing" {
		t.Fatal("wrapped NoSuchKey was not translated to PathNotFoundError")
	}
	if !errors.Is(parseError("/missing", context.Canceled), context.Canceled) {
		t.Fatal("context cancellation was not preserved")
	}
}

func TestSDKV2RedirectValidation(t *testing.T) {
	isolateAWSConfig(t)
	for _, endpoint := range []string{
		"download.example", "https://", "https://download.example/base", "https://download.example?query=value",
		"ftp://download.example", "https://download.example?", "https://user:password@download.example", "https://download.example#fragment",
	} {
		t.Run(endpoint, func(t *testing.T) {
			params := sdkTestParameters("https://storage.example")
			params["redirectendpoint"] = endpoint
			if _, err := FromParameters(t.Context(), params); err == nil {
				t.Fatal("invalid redirect endpoint was accepted")
			}
		})
	}
	params := sdkTestParameters("https://storage.example")
	d := driverFromParams(t, params)
	redirect, err := d.RedirectURL(httptest.NewRequest(http.MethodPut, "http://registry.example", nil), "/key")
	if err != nil || redirect != "" {
		t.Fatalf("unsupported method redirect = %q, %v", redirect, err)
	}
}

func TestSDKV2ReaderInvalidRange(t *testing.T) {
	isolateAWSConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=42-" {
			t.Error("reader did not preserve the byte offset")
		}
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		_, _ = io.WriteString(w, "<Error><Code>InvalidRange</Code></Error>")
	}))
	defer server.Close()
	d := driverFromParams(t, sdkTestParameters(server.URL))
	reader, err := d.Reader(t.Context(), "/key", 42)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil || len(body) != 0 {
		t.Fatalf("out-of-range read = %q, %v; want EOF", body, err)
	}
}

func TestSDKV2StatFallback(t *testing.T) {
	isolateAWSConfig(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet || r.URL.Query().Get("prefix") != "dir" {
			t.Error("stat must fall back to listing the requested prefix")
		}
		_, _ = io.WriteString(w, "<ListBucketResult><Contents><Key>dir/key</Key></Contents></ListBucketResult>")
	}))
	defer server.Close()
	d := driverFromParams(t, sdkTestParameters(server.URL))
	info, err := d.Stat(t.Context(), "/dir")
	if err != nil || !info.IsDir() || requests.Load() != 2 {
		t.Fatalf("stat fallback = %v, %v; requests = %d", info, err, requests.Load())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Stat(ctx, "/dir"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled stat = %v", err)
	}
	if requests.Load() != 2 {
		t.Fatal("canceled stat sent a request")
	}
}

func TestSDKV2WalkPagination(t *testing.T) {
	isolateAWSConfig(t)
	walkErr := errors.New("stop walking")
	for _, tc := range []struct {
		name        string
		callbackErr error
		wantErr     error
		paths       []string
		pages       int32
	}{
		{"all", nil, nil, []string{"/a", "/a/one", "/b", "/b/two"}, 2},
		{"skip directory", storagedriver.ErrSkipDir, nil, []string{"/a", "/b", "/b/two"}, 2},
		{"filled buffer", storagedriver.ErrFilledBuffer, nil, []string{"/a"}, 1},
		{"callback error", walkErr, walkErr, []string{"/a"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				q := r.URL.Query()
				if q.Get("prefix") != "root/" || q.Get("start-after") != "root/0" || q.Get("max-keys") != "1000" {
					t.Errorf("unexpected walk parameters: %s", r.URL.RawQuery)
				}
				key, more := "root/a/one", "<IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>"
				if q.Get("continuation-token") == "next" {
					key, more = "root/b/two", "<IsTruncated>false</IsTruncated>"
				}
				_, _ = fmt.Fprintf(w, "<ListBucketResult>%s<Contents><Key>%s</Key><Size>1</Size><LastModified>2026-09-04T00:00:00Z</LastModified></Contents></ListBucketResult>", more, key)
			}))
			defer server.Close()
			params := sdkTestParameters(server.URL)
			params["rootdirectory"] = "root"
			d := driverFromParams(t, params)
			var paths []string
			err := d.Walk(t.Context(), "/", func(info storagedriver.FileInfo) error {
				paths = append(paths, info.Path())
				if info.Path() == "/a" {
					return tc.callbackErr
				}
				return nil
			}, func(o *storagedriver.WalkOptions) { o.StartAfterHint = "/0" })
			if !errors.Is(err, tc.wantErr) || !slices.Equal(paths, tc.paths) || requests.Load() != tc.pages {
				t.Fatalf("walk = %v, %v, %d pages; want %v, %v, %d pages", paths, err, requests.Load(), tc.paths, tc.wantErr, tc.pages)
			}
		})
	}
}

func TestSDKV2MultipartUploadPagination(t *testing.T) {
	isolateAWSConfig(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q := r.URL.Query()
		if !q.Has("uploads") || q.Get("prefix") != "key" {
			t.Errorf("unexpected multipart listing: %s", r.URL.RawQuery)
		}
		more := "<IsTruncated>true</IsTruncated><NextKeyMarker>key-suffix</NextKeyMarker><NextUploadIdMarker>first</NextUploadIdMarker>"
		if requests.Load() == 2 {
			if q.Get("key-marker") != "key-suffix" || q.Get("upload-id-marker") != "first" {
				t.Error("multipart pagination must carry both markers")
			}
			more = "<IsTruncated>false</IsTruncated>"
		}
		_, _ = fmt.Fprintf(w, "<ListMultipartUploadsResult>%s<Upload><Key>key-suffix</Key><UploadId>other</UploadId></Upload></ListMultipartUploadsResult>", more)
	}))
	defer server.Close()
	d := driverFromParams(t, sdkTestParameters(server.URL))
	_, err := d.Writer(t.Context(), "/key", true)
	var missing storagedriver.PathNotFoundError
	if !errors.As(err, &missing) || missing.Path != "/key" || requests.Load() != 2 {
		t.Fatalf("resume missing upload = %v; requests = %d", err, requests.Load())
	}
}

func TestSDKV2MultipartPartPagination(t *testing.T) {
	isolateAWSConfig(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q := r.URL.Query()
		if q.Has("uploads") {
			_, _ = io.WriteString(w, "<ListMultipartUploadsResult><Upload><Key>key</Key><UploadId>upload</UploadId></Upload></ListMultipartUploadsResult>")
			return
		}
		if q.Get("uploadId") != "upload" {
			t.Error("listing parts of the wrong upload")
		}
		part, size := 1, minChunkSize
		more := "<IsTruncated>true</IsTruncated><NextPartNumberMarker>1</NextPartNumberMarker>"
		if q.Get("part-number-marker") == "1" {
			part, size, more = 2, 3, "<IsTruncated>false</IsTruncated>"
		}
		_, _ = fmt.Fprintf(w, "<ListPartsResult>%s<Part><PartNumber>%d</PartNumber><Size>%d</Size><ETag>etag</ETag></Part></ListPartsResult>", more, part, size)
	}))
	defer server.Close()
	d := driverFromParams(t, sdkTestParameters(server.URL))
	w, err := d.Writer(t.Context(), "/key", true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Size() != minChunkSize+3 || len(w.(*writer).parts) != 2 || requests.Load() != 3 {
		t.Fatalf("resumed size = %d, parts = %d, requests = %d", w.Size(), len(w.(*writer).parts), requests.Load())
	}
}

func TestSDKV2RootStat(t *testing.T) {
	isolateAWSConfig(t)
	for _, root := range []string{"", "/"} {
		for _, empty := range []bool{true, false} {
			t.Run(root+strconv.FormatBool(empty), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Query().Get("list-type") != "2" || r.URL.Query().Get("prefix") != "" {
						t.Error("root stat must list the bucket")
					}
					body := "<ListBucketResult>"
					if !empty {
						body += "<Contents><Key>object</Key><Size>1</Size><LastModified>2026-09-04T00:00:00Z</LastModified></Contents>"
					}
					_, _ = io.WriteString(w, body+"</ListBucketResult>")
				}))
				defer server.Close()
				params := sdkTestParameters(server.URL)
				params["rootdirectory"] = root
				d := driverFromParams(t, params)
				info, err := d.Stat(t.Context(), "/")
				if empty {
					var missing storagedriver.PathNotFoundError
					if !errors.As(err, &missing) || missing.Path != "/" {
						t.Fatalf("empty bucket health check: %v", err)
					}
				} else if err != nil || !info.IsDir() || info.Path() != "/" {
					t.Fatalf("populated bucket root stat = %v, %v", info, err)
				}
			})
		}
	}
}

// TestRedirectEndpointIntegration checks that the public authority is used in
// the actual signature, by fetching through a second address of the S3 server.
func TestRedirectEndpointIntegration(t *testing.T) {
	skipCheck(t)
	endpoint := os.Getenv("S3_REDIRECT_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_REDIRECT_ENDPOINT is required for public endpoint integration")
	}
	drv, err := s3DriverConstructor(t.TempDir(), noStorageClass, func(p *DriverParameters) {
		p.RedirectEndpoint = endpoint
	})
	if err != nil {
		t.Fatal(err)
	}
	d := drv.StorageDriver.(*driver)
	publicEndpoint, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	const key = "/public-download"
	if err := d.PutContent(t.Context(), key, []byte("public payload")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Delete(context.Background(), key); err != nil {
			t.Error(err)
		}
	})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		signed, err := d.RedirectURL(httptest.NewRequest(method, "http://registry.invalid", nil), key)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(signed)
		if err != nil || u.Host != publicEndpoint.Host {
			t.Fatalf("presign did not select the public endpoint: %v", err)
		}
		request, err := http.NewRequestWithContext(t.Context(), method, signed, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil {
			t.Fatalf("public %s failed: status %d, error %v", method, response.StatusCode, readErr)
		}
		if method == http.MethodGet && string(body) != "public payload" {
			t.Fatal("public download payload differs")
		}
	}
}
