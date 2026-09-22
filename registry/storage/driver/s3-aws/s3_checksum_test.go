package s3

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestDeleteObjectsChecksumMiddleware(t *testing.T) {
	for _, operation := range []string{"DeleteObjects", "PutObject", "UploadPart", "GetObject"} {
		t.Run(operation, func(t *testing.T) {
			stack := middleware.NewStack(operation, smithyhttp.NewStackRequest)
			if err := addDeleteObjectsContentMD5(stack); err != nil {
				t.Fatal(err)
			}
			m, installed := stack.Build.Get("ContentChecksum")
			if installed != (operation == "DeleteObjects") {
				t.Fatalf("ContentChecksum installed=%v", installed)
			}
			if !installed {
				return
			}
			for _, existing := range []string{"", "existing-checksum"} {
				const body = "<Delete><Object><Key>key</Key></Object></Delete>"
				req, err := smithyhttp.NewStackRequest().(*smithyhttp.Request).SetStream(strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-MD5", existing)
				want := existing
				if want == "" {
					sum := md5.Sum([]byte(body))
					want = base64.StdEncoding.EncodeToString(sum[:])
				}
				called := false
				_, _, err = m.HandleBuild(t.Context(), middleware.BuildInput{Request: req}, middleware.BuildHandlerFunc(func(_ context.Context, in middleware.BuildInput) (middleware.BuildOutput, middleware.Metadata, error) {
					called = true
					request := in.Request.(*smithyhttp.Request)
					if got := request.Header.Get("Content-MD5"); got != want {
						t.Errorf("Content-MD5=%q, want %q", got, want)
					}
					got, err := io.ReadAll(request.GetStream())
					if err != nil || string(got) != body {
						t.Errorf("body not preserved: %q, error=%v", got, err)
					}
					return middleware.BuildOutput{}, middleware.Metadata{}, nil
				}))
				if err != nil || !called {
					t.Fatalf("middleware failed to forward request: called=%v, error=%v", called, err)
				}
			}
		})
	}
}

func TestUploadChecksumWireFormat(t *testing.T) {
	isolateAWSConfig(t)
	for _, scheme := range []string{"http", "https"} {
		for _, policy := range []string{"default", "when_required", "when_supported"} {
			t.Run(scheme+"/"+policy, func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if r.Method != http.MethodPut || r.Header.Get("Authorization") == "" {
						t.Error("expected a signed upload")
					}
					if policy != "when_supported" {
						assertUploadContentChecksums(t, r, body)
						for _, name := range []string{"X-Amz-Checksum-Crc32", "X-Amz-Sdk-Checksum-Algorithm", "X-Amz-Trailer", "Content-Encoding"} {
							if got := r.Header.Get(name); got != "" {
								t.Errorf("unexpected %s = %q", name, got)
							}
						}
						if string(body) != "payload" || r.ContentLength != 7 {
							t.Errorf("expected unframed payload, got %q (length %d)", body, r.ContentLength)
						}
					} else if scheme == "https" {
						if r.Header.Get("Content-Encoding") != "aws-chunked" || !strings.Contains(string(body), "x-amz-checksum-crc32:") {
							t.Errorf("missing TLS checksum framing: %q", body)
						}
					} else if r.Header.Get("X-Amz-Checksum-Crc32") == "" {
						t.Error("missing enabled CRC32 checksum")
					}
					w.Header().Set("ETag", `"etag"`)
					if policy == "when_supported" && r.Header.Get("Content-MD5") != "" {
						t.Error("compatibility MD5 applied to when_supported")
					}
				}))
				if scheme == "https" {
					server.StartTLS()
				} else {
					server.Start()
				}
				defer server.Close()
				params := sdkTestParameters(server.URL)
				params["skipverify"] = true
				if policy != "default" {
					params["requestchecksumcalculation"] = policy
				}
				d := driverFromParams(t, params)
				if err := d.PutContent(t.Context(), "/key", []byte("payload")); err != nil {
					t.Fatal(err)
				}
				if _, err := d.S3.UploadPart(t.Context(), &awss3.UploadPartInput{
					Bucket: aws.String(d.Bucket), Key: aws.String("key"), UploadId: aws.String("upload"),
					PartNumber: aws.Int32(1), Body: strings.NewReader("payload"),
				}); err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 2 {
					t.Fatalf("requests = %d, want PutObject and UploadPart", requests.Load())
				}
			})
		}
	}
}

func assertUploadContentChecksums(t *testing.T, r *http.Request, body []byte) {
	t.Helper()
	md5Sum := md5.Sum(body)
	if got, want := r.Header.Get("Content-MD5"), base64.StdEncoding.EncodeToString(md5Sum[:]); got != want {
		t.Errorf("Content-MD5 = %q, want %q", got, want)
	}
	sha256Sum := sha256.Sum256(body)
	if got, want := r.Header.Get("X-Amz-Content-Sha256"), hex.EncodeToString(sha256Sum[:]); got != want {
		t.Errorf("payload hash = %q, want %q", got, want)
	}
	_, signed, _ := strings.Cut(r.Header.Get("Authorization"), "SignedHeaders=")
	signed, _, _ = strings.Cut(signed, ",")
	if !slices.Contains(strings.Split(signed, ";"), "content-md5") {
		t.Error("Content-MD5 is not signed")
	}
}

func TestUploadContentChecksumsOnRetry(t *testing.T) {
	isolateAWSConfig(t)
	for _, scheme := range []string{"http", "https"} {
		for _, operation := range []string{"PutObject", "UploadPart"} {
			for _, tc := range []struct{ name, payload string }{{"empty", ""}, {"binary", "payload\x00\xff"}} {
				t.Run(scheme+"/"+operation+"/"+tc.name, func(t *testing.T) {
					payload := tc.payload
					var attempts atomic.Int32
					server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil || string(body) != payload {
							t.Errorf("upload body = %q, error = %v", body, err)
						}
						assertUploadContentChecksums(t, r, body)
						if attempts.Add(1) == 1 {
							w.WriteHeader(http.StatusTooManyRequests)
							return
						}
						w.Header().Set("ETag", `"etag"`)
					}))
					if scheme == "https" {
						server.StartTLS()
					} else {
						server.Start()
					}
					defer server.Close()
					params := sdkTestParameters(server.URL)
					params["skipverify"] = true
					d := driverFromParams(t, params)
					options := d.S3.Options()
					options.Retryer = retry.AddWithMaxBackoffDelay(options.Retryer, time.Nanosecond)
					d.S3 = awss3.New(options)
					var err error
					if operation == "PutObject" {
						err = d.PutContent(t.Context(), "/key", []byte(payload))
					} else {
						_, err = d.S3.UploadPart(t.Context(), &awss3.UploadPartInput{
							Bucket: aws.String(d.Bucket), Key: aws.String("key"), UploadId: aws.String("upload"),
							PartNumber: aws.Int32(1), Body: strings.NewReader(payload),
						})
					}
					if err != nil || attempts.Load() != 2 {
						t.Fatalf("upload attempts = %d, error = %v", attempts.Load(), err)
					}
				})
			}
		}
	}
}
