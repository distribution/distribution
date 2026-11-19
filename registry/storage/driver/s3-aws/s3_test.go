package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
)

var (
	s3DriverConstructor func(rootDirectory, storageClass string) (*Driver, error)
	skipCheck           func(tb testing.TB)
)

func init() {
	var (
		accessKey       = os.Getenv("AWS_ACCESS_KEY")
		secretKey       = os.Getenv("AWS_SECRET_KEY")
		bucket          = os.Getenv("S3_BUCKET")
		encrypt         = os.Getenv("S3_ENCRYPT")
		keyID           = os.Getenv("S3_KEY_ID")
		secure          = os.Getenv("S3_SECURE")
		skipVerify      = os.Getenv("S3_SKIP_VERIFY")
		v4Auth          = os.Getenv("S3_V4_AUTH")
		region          = os.Getenv("AWS_REGION")
		objectACL       = os.Getenv("S3_OBJECT_ACL")
		regionEndpoint  = os.Getenv("REGION_ENDPOINT")
		forcePathStyle  = os.Getenv("AWS_S3_FORCE_PATH_STYLE")
		sessionToken    = os.Getenv("AWS_SESSION_TOKEN")
		useDualStack    = os.Getenv("S3_USE_DUALSTACK")
		accelerate      = os.Getenv("S3_ACCELERATE")
		useFIPSEndpoint = os.Getenv("S3_USE_FIPS_ENDPOINT")
		logLevel        = os.Getenv("S3_LOGLEVEL")
	)

	var err error
	s3DriverConstructor = func(rootDirectory, storageClass string) (*Driver, error) {
		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		secureBool := true
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		skipVerifyBool := false
		if skipVerify != "" {
			skipVerifyBool, err = strconv.ParseBool(skipVerify)
			if err != nil {
				return nil, err
			}
		}

		v4Bool := true
		if v4Auth != "" {
			v4Bool, err = strconv.ParseBool(v4Auth)
			if err != nil {
				return nil, err
			}
		}
		forcePathStyleBool := false
		if forcePathStyle != "" {
			forcePathStyleBool, err = strconv.ParseBool(forcePathStyle)
			if err != nil {
				return nil, err
			}
		}

		useDualStackBool := false
		if useDualStack != "" {
			useDualStackBool, err = strconv.ParseBool(useDualStack)
		}

		accelerateBool := true
		if accelerate != "" {
			accelerateBool, err = strconv.ParseBool(accelerate)
			if err != nil {
				return nil, err
			}
		}

		useFIPSEndpointBool := false
		if useFIPSEndpoint != "" {
			useFIPSEndpointBool, err = strconv.ParseBool(useFIPSEndpoint)
			if err != nil {
				return nil, err
			}
		}

		if objectACL == "" {
			objectACL = s3.ObjectCannedACLPrivate
		}

		parameters := DriverParameters{
			AccessKey:                   accessKey,
			SecretKey:                   secretKey,
			Bucket:                      bucket,
			Region:                      region,
			RegionEndpoint:              regionEndpoint,
			ForcePathStyle:              forcePathStyleBool,
			Encrypt:                     encryptBool,
			KeyID:                       keyID,
			Secure:                      secureBool,
			SkipVerify:                  skipVerifyBool,
			V4Auth:                      v4Bool,
			ChunkSize:                   minChunkSize,
			MultipartCopyChunkSize:      defaultMultipartCopyChunkSize,
			MultipartCopyMaxConcurrency: defaultMultipartCopyMaxConcurrency,
			MultipartCopyThresholdSize:  defaultMultipartCopyThresholdSize,
			RootDirectory:               rootDirectory,
			StorageClass:                storageClass,
			UserAgent:                   driverName + "-test",
			ObjectACL:                   objectACL,
			SessionToken:                sessionToken,
			UseDualStack:                useDualStackBool,
			Accelerate:                  accelerateBool,
			UseFIPSEndpoint:             useFIPSEndpointBool,
			LogLevel:                    getS3LogLevelFromParam(logLevel),
		}

		return New(context.Background(), parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck = func(tb testing.TB) {
		tb.Helper()

		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			tb.Skip("Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests")
		}
	}
}

func newDriverConstructor(tb testing.TB) testsuites.DriverConstructor {
	root := tb.TempDir()

	return func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(root, s3.StorageClassStandard)
	}
}

func TestS3DriverSuite(t *testing.T) {
	skipCheck(t)
	skipVerify, err := strconv.ParseBool(os.Getenv("S3_SKIP_VERIFY"))
	if err != nil {
		skipVerify = false
	}
	testsuites.Driver(t, newDriverConstructor(t), skipVerify)
}

func BenchmarkS3DriverSuite(b *testing.B) {
	skipCheck(b)
	testsuites.BenchDriver(b, newDriverConstructor(b))
}

func TestNewBufferPoolAllocatesLazily(t *testing.T) {
	buf := newBufferPool().Get().(*bytes.Buffer)
	if buf.Len() != 0 || buf.Cap() != 0 {
		t.Fatalf("new pooled buffer has len %d and cap %d, want both zero", buf.Len(), buf.Cap())
	}
}

func TestWriterWriteBufferGrowth(t *testing.T) {
	const chunkSize = 1024

	tests := []struct {
		name    string
		writes  [][]byte
		wantLen int
		wantCap int
		wantErr error
	}{
		{
			name:    "empty write does not allocate",
			wantLen: 0,
			wantCap: 0,
		},
		{
			name:    "small write grows to data",
			writes:  [][]byte{make([]byte, 100)},
			wantLen: 100,
			wantCap: 100,
		},
		{
			name:    "subsequent write grows geometrically",
			writes:  [][]byte{make([]byte, 100), make([]byte, 100)},
			wantLen: 200,
			wantCap: 800,
		},
		{
			name:    "large write caps growth at chunk size",
			writes:  [][]byte{make([]byte, 100), make([]byte, 100), make([]byte, 824)},
			wantLen: chunkSize,
			wantCap: chunkSize,
		},
		{
			name:    "write past chunk is rejected",
			writes:  [][]byte{make([]byte, chunkSize), {0}},
			wantLen: chunkSize,
			wantCap: chunkSize,
			wantErr: io.ErrShortBuffer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &writer{
				driver: &driver{ChunkSize: chunkSize},
				buf:    newBufferPool().Get().(*bytes.Buffer),
			}

			var err error
			for _, data := range tt.writes {
				if _, err = w.writeBuffer(data); err != nil {
					break
				}
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("writeBuffer() error = %v, want %v", err, tt.wantErr)
			}
			if w.buf.Len() != tt.wantLen {
				t.Errorf("buffer length = %d, want %d", w.buf.Len(), tt.wantLen)
			}
			if w.buf.Cap() != tt.wantCap {
				t.Errorf("buffer capacity = %d, want %d", w.buf.Cap(), tt.wantCap)
			}
		})
	}
}

func TestWriterWriteBufferWithMaximumChunkSize(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	w := &writer{
		driver: &driver{ChunkSize: maxInt},
		buf:    newBufferPool().Get().(*bytes.Buffer),
	}

	n, err := w.writeBuffer([]byte{0})
	if err != nil {
		t.Fatalf("writeBuffer() error: %v", err)
	}
	if n != 1 {
		t.Errorf("writeBuffer() wrote %d bytes, want 1", n)
	}
	if w.buf.Cap() != 1 {
		t.Errorf("buffer capacity = %d, want 1", w.buf.Cap())
	}
}

func TestWriterWriteBoundsBufferToChunkSize(t *testing.T) {
	var uploadedPartSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		part, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading uploaded part: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		uploadedPartSizes = append(uploadedPartSizes, len(part))
		w.Header().Set("ETag", fmt.Sprintf(`"part-%d"`, len(uploadedPartSizes)))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials("access-key", "secret-key", ""),
		DisableSSL:       aws.Bool(true),
		Endpoint:         aws.String(server.URL),
		Region:           aws.String("us-east-1"),
		S3ForcePathStyle: aws.Bool(true),
	}))

	const chunkSize = 8
	d := &driver{
		S3:        s3.New(sess),
		Bucket:    "bucket",
		ChunkSize: chunkSize,
		pool:      newBufferPool(),
	}
	w := d.newWriter(t.Context(), "key", "upload-id", nil).(*writer)

	payload := bytes.Repeat([]byte{0xab}, 2*chunkSize+5)
	n, err := w.Write(payload)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write() wrote %d bytes, want %d", n, len(payload))
	}
	if w.buf.Cap() > chunkSize {
		t.Errorf("buffer capacity = %d, want at most %d", w.buf.Cap(), chunkSize)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	wantPartSizes := []int{chunkSize, chunkSize, 5}
	if !slices.Equal(uploadedPartSizes, wantPartSizes) {
		t.Errorf("uploaded part sizes = %v, want %v", uploadedPartSizes, wantPartSizes)
	}
}

func TestWriterWriteExcludesFailedPartFromCount(t *testing.T) {
	var mu sync.Mutex
	var uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("reading uploaded part: %v", err)
		}
		mu.Lock()
		uploads++
		attempt := uploads
		mu.Unlock()
		if attempt > 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("ETag", `"part-1"`)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials("access-key", "secret-key", ""),
		DisableSSL:       aws.Bool(true),
		Endpoint:         aws.String(server.URL),
		Region:           aws.String("us-east-1"),
		S3ForcePathStyle: aws.Bool(true),
	}))

	const chunkSize = 8
	d := &driver{
		S3:        s3.New(sess),
		Bucket:    "bucket",
		ChunkSize: chunkSize,
		pool:      newBufferPool(),
	}
	w := d.newWriter(t.Context(), "key", "upload-id", nil).(*writer)

	n, err := w.Write(bytes.Repeat([]byte{0xab}, 2*chunkSize+5))
	if err == nil {
		t.Fatal("Write() succeeded, want error from failed part upload")
	}
	if n != chunkSize {
		t.Errorf("Write() wrote %d bytes, want %d", n, chunkSize)
	}
	if w.Size() != chunkSize {
		t.Errorf("Size() = %d, want %d", w.Size(), chunkSize)
	}
}

func TestWriterResumeCopy(t *testing.T) {
	const copyLimit int64 = 5 * 1024 * 1024 * 1024
	for _, size := range []int64{
		copyLimit - 1, copyLimit, copyLimit + 1,
		copyLimit + minChunkSize - 1, copyLimit + minChunkSize,
		2 * copyLimit, 2*copyLimit + 1,
	} {
		t.Run(strconv.FormatInt(size, 10), func(t *testing.T) {
			var copied int64
			var parts []*s3.CompletedPart
			var committed bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				switch {
				case r.Method == http.MethodGet && q.Has("uploads"):
					fmt.Fprint(w, `<ListMultipartUploadsResult><IsTruncated>false</IsTruncated><Upload><Key>key</Key><UploadId>old</UploadId></Upload></ListMultipartUploadsResult>`)
				case r.Method == http.MethodGet && q.Get("uploadId") == "old":
					// Only metadata is needed: no multi-GiB payload is allocated.
					// Paginate just as S3 does when a history has over 1000 parts.
					marker, _ := strconv.Atoi(q.Get("part-number-marker"))
					const partSize = 6 * 1024 * 1024
					count := int((size + partSize - 1) / partSize)
					end := min(marker+1000, count)
					fmt.Fprintf(w, `<ListPartsResult><IsTruncated>%t</IsTruncated><NextPartNumberMarker>%d</NextPartNumberMarker>`, end < count, end)
					for i := marker; i < end; i++ {
						fmt.Fprintf(w, `<Part><PartNumber>%d</PartNumber><Size>%d</Size><ETag>old-%d</ETag></Part>`, i+1, min(size-int64(i)*partSize, partSize), i+1)
					}
					fmt.Fprint(w, `</ListPartsResult>`)
				case r.Method == http.MethodPost && q.Get("uploadId") == "old":
					fmt.Fprint(w, `<CompleteMultipartUploadResult><ETag>saved</ETag></CompleteMultipartUploadResult>`)
				case r.Method == http.MethodPost && q.Has("uploads"):
					fmt.Fprint(w, `<InitiateMultipartUploadResult><UploadId>new</UploadId></InitiateMultipartUploadResult>`)
				case r.Method == http.MethodPut && q.Get("uploadId") == "new":
					number := int64(len(parts) + 1)
					if q.Get("partNumber") != strconv.FormatInt(number, 10) || number > 10000 {
						t.Errorf("part number = %q, want %d in 1..10000", q.Get("partNumber"), number)
					}
					if r.Header.Get("X-Amz-Copy-Source") != "" {
						if got := r.Header.Get("X-Amz-Copy-Source"); got != "bucket/key" {
							t.Errorf("copy source = %q, want bucket/key", got)
						}
						rangeHeader := r.Header.Get("X-Amz-Copy-Source-Range")
						first, last := int64(0), size-1
						if rangeHeader != "" {
							if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &first, &last); err != nil {
								t.Errorf("invalid copy range %q: %v", rangeHeader, err)
							}
						}
						if last-first+1 > copyLimit {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprint(w, `<Error><Code>EntityTooLarge</Code><Message>copy part exceeds 5 GiB</Message></Error>`)
							return
						}
						if first != copied || last < first || last >= size || last-first+1 < minChunkSize {
							t.Errorf("copy range %d-%d after %d bytes of %d does not preserve an appendable prefix", first, last, copied, size)
						}
						copied = last + 1
					} else {
						body, err := io.ReadAll(r.Body)
						if err != nil || string(body) != "tail" || copied != size {
							t.Errorf("appended body = %q, err=%v, copied=%d, want tail after %d", body, err, copied, size)
						}
					}
					etag := fmt.Sprintf("new-%d", number)
					parts = append(parts, &s3.CompletedPart{ETag: aws.String(etag), PartNumber: aws.Int64(number)})
					w.Header().Set("ETag", etag)
					fmt.Fprintf(w, `<CopyPartResult><ETag>%s</ETag></CopyPartResult>`, etag)
				case r.Method == http.MethodPost && q.Get("uploadId") == "new":
					var completion struct {
						Parts []struct {
							ETag       string
							PartNumber int64
						} `xml:"Part"`
					}
					if err := xml.NewDecoder(r.Body).Decode(&completion); err != nil {
						t.Errorf("decode completion: %v", err)
					}
					if len(completion.Parts) != len(parts) {
						t.Errorf("completion has %d parts, want %d", len(completion.Parts), len(parts))
					} else {
						for i, part := range completion.Parts {
							if part.ETag != *parts[i].ETag || part.PartNumber != *parts[i].PartNumber {
								t.Errorf("completed part %d = %+v, want %v", i, part, parts[i])
							}
						}
					}
					committed = true
					fmt.Fprint(w, `<CompleteMultipartUploadResult><ETag>done</ETag></CompleteMultipartUploadResult>`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			sess := session.Must(session.NewSession(&aws.Config{
				Credentials:      credentials.NewStaticCredentials("access-key", "secret-key", ""),
				Endpoint:         aws.String(server.URL),
				Region:           aws.String("us-east-1"),
				S3ForcePathStyle: aws.Bool(true),
			}))
			d := &driver{S3: s3.New(sess), Bucket: "bucket", ChunkSize: defaultChunkSize, pool: newBufferPool()}
			w, err := d.Writer(t.Context(), "/key", true)
			if err != nil {
				t.Fatal(err)
			}
			if w.Size() != size {
				t.Fatalf("initial size = %d, want %d", w.Size(), size)
			}
			for _, data := range [][]byte{[]byte("ta"), nil, []byte("il")} {
				n, err := w.Write(data)
				if err != nil || n != len(data) {
					t.Fatalf("resumed write: n=%d err=%v", n, err)
				}
			}
			if err := w.Commit(t.Context()); err != nil {
				t.Fatalf("resumed commit: %v", err)
			}
			if copied != size || w.Size() != size+4 || !committed {
				t.Errorf("resumed prefix: copied=%d size=%d committed=%t, want %d %d true", copied, w.Size(), committed, size, size+4)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWriterResumeCopyFailure(t *testing.T) {
	const size int64 = 5*1024*1024*1024 + 1
	for _, failPart := range []int{1, 2} {
		t.Run(strconv.Itoa(failPart), func(t *testing.T) {
			var aborted bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					if r.URL.Query().Has("uploads") {
						fmt.Fprint(w, `<InitiateMultipartUploadResult><UploadId>new</UploadId></InitiateMultipartUploadResult>`)
					} else {
						fmt.Fprint(w, `<CompleteMultipartUploadResult><ETag>saved</ETag></CompleteMultipartUploadResult>`)
					}
				case http.MethodPut:
					if r.URL.Query().Get("partNumber") == strconv.Itoa(failPart) {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>copy denied</Message></Error>`)
						return
					}
					fmt.Fprint(w, `<CopyPartResult><ETag>copied</ETag></CopyPartResult>`)
				case http.MethodDelete:
					if got := r.URL.Query().Get("uploadId"); got != "new" {
						t.Errorf("aborted upload = %q, want new", got)
					}
					aborted = true
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			sess := session.Must(session.NewSession(&aws.Config{
				Credentials:      credentials.NewStaticCredentials("access-key", "secret-key", ""),
				Endpoint:         aws.String(server.URL),
				Region:           aws.String("us-east-1"),
				S3ForcePathStyle: aws.Bool(true),
			}))
			d := &driver{S3: s3.New(sess), Bucket: "bucket", ChunkSize: defaultChunkSize, pool: newBufferPool()}
			parts := []*s3.Part{
				{ETag: aws.String("old-1"), PartNumber: aws.Int64(1), Size: aws.Int64(size - 1)},
				{ETag: aws.String("old-2"), PartNumber: aws.Int64(2), Size: aws.Int64(1)},
			}
			w := d.newWriter(t.Context(), "key", "old", parts).(*writer)
			n, err := w.Write([]byte("tail"))
			var s3err awserr.Error
			if !errors.As(err, &s3err) || s3err.Code() != "AccessDenied" {
				t.Fatalf("copy failure = %v, want AccessDenied", err)
			}
			if n != 0 || w.Size() != size || w.buf.Len() != 0 || w.parts[0] != parts[0] || !aborted {
				t.Errorf("failed copy: n=%d size=%d buffer=%d parts=%v aborted=%t", n, w.Size(), w.buf.Len(), w.parts, aborted)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func BenchmarkWriterBufferGrowth(b *testing.B) {
	const (
		chunkSize = 10 * 1024 * 1024
		writeSize = 32 * 1024
	)
	data := make([]byte, writeSize)

	b.ReportAllocs()
	for b.Loop() {
		w := &writer{
			driver: &driver{ChunkSize: chunkSize},
			buf:    newBufferPool().Get().(*bytes.Buffer),
		}
		for w.buf.Len() < chunkSize {
			if _, err := w.writeBuffer(data); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestGetS3LogLevelFromParam(t *testing.T) {
	tests := []struct {
		name     string
		param    any
		expected aws.LogLevelType
	}{
		// Nil case
		{
			name:     "nil parameter",
			param:    nil,
			expected: aws.LogOff,
		},

		// Boolean cases - YAML converts "off" to false
		{
			name:     "boolean false (from YAML off)",
			param:    false,
			expected: aws.LogOff,
		},
		{
			name:     "boolean true",
			param:    true,
			expected: aws.LogOff,
		},

		// String cases - valid values
		{
			name:     "string off",
			param:    "off",
			expected: aws.LogOff,
		},
		{
			name:     "string OFF uppercase",
			param:    "OFF",
			expected: aws.LogOff,
		},
		{
			name:     "string debug",
			param:    "debug",
			expected: aws.LogDebug,
		},
		{
			name:     "string DEBUG uppercase",
			param:    "DEBUG",
			expected: aws.LogDebug,
		},
		{
			name:     "string debugwithsigning",
			param:    "debugwithsigning",
			expected: aws.LogDebugWithSigning,
		},
		{
			name:     "string debugwithhttpbody",
			param:    "debugwithhttpbody",
			expected: aws.LogDebugWithHTTPBody,
		},
		{
			name:     "string debugwithrequestretries",
			param:    "debugwithrequestretries",
			expected: aws.LogDebugWithRequestRetries,
		},
		{
			name:     "string debugwithrequesterrors",
			param:    "debugwithrequesterrors",
			expected: aws.LogDebugWithRequestErrors,
		},
		{
			name:     "string debugwitheventstreambody",
			param:    "debugwitheventstreambody",
			expected: aws.LogDebugWithEventStreamBody,
		},

		// Invalid cases
		{
			name:     "invalid string value",
			param:    "invalid",
			expected: aws.LogOff,
		},
		{
			name:     "empty string",
			param:    "",
			expected: aws.LogOff,
		},
		{
			name:     "integer value",
			param:    42,
			expected: aws.LogOff,
		},
		{
			name:     "float value",
			param:    42.0,
			expected: aws.LogOff,
		},
		{
			name:     "slice value",
			param:    []string{"off"},
			expected: aws.LogOff,
		},
		{
			name:     "map value",
			param:    map[string]string{"level": "off"},
			expected: aws.LogOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getS3LogLevelFromParam(tt.param)
			if result != tt.expected {
				t.Errorf("getS3LogLevelFromParam(%v) = %v, want %v", tt.param, result, tt.expected)
			}
		})
	}
}

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		parent   string
		expected bool
	}{
		{
			name:     "empty parent",
			path:     "/tags/1.2",
			parent:   "",
			expected: false,
		},
		{
			name:     "same path",
			path:     "/tags/1.2",
			parent:   "/tags/1.2",
			expected: true,
		},
		{
			name:     "descendant path",
			path:     "/tags/1.2/current/link",
			parent:   "/tags/1.2",
			expected: true,
		},
		{
			name:     "sibling with lexical prefix",
			path:     "/tags/1.20/current/link",
			parent:   "/tags/1.2",
			expected: false,
		},
		{
			name:     "root parent",
			path:     "/tags/1.2",
			parent:   "/",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := isSubpath(tt.path, tt.parent)
			if actual != tt.expected {
				t.Fatalf("isSubpath(%q, %q) = %t, want %t", tt.path, tt.parent, actual, tt.expected)
			}
		})
	}
}

func TestDirectoryDiff(t *testing.T) {
	tests := []struct {
		name     string
		prev     string
		current  string
		expected []string
	}{
		{
			name:     "immediate child",
			prev:     "/path/to/folder",
			current:  "/path/to/folder/folder/file",
			expected: []string{"/path/to/folder/folder"},
		},
		{
			name:     "sibling folder",
			prev:     "/path/to/folder/folder1",
			current:  "/path/to/folder/folder2/file",
			expected: []string{"/path/to/folder/folder2"},
		},
		{
			name:     "sibling folder, prev is a file",
			prev:     "/path/to/folder/folder1/file",
			current:  "/path/to/folder/folder2/file",
			expected: []string{"/path/to/folder/folder2"},
		},
		{
			name:     "multiple missing directories",
			prev:     "/path/to/folder/folder1/file",
			current:  "/path/to/folder/folder2/folder1/file",
			expected: []string{"/path/to/folder/folder2", "/path/to/folder/folder2/folder1"},
		},
		{
			name:     "from root, deeply nested",
			prev:     "/",
			current:  "/path/to/folder/folder/file",
			expected: []string{"/path", "/path/to", "/path/to/folder", "/path/to/folder/folder"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := directoryDiff(tt.prev, tt.current)
			if !slices.Equal(actual, tt.expected) {
				t.Fatalf("directoryDiff(%q, %q) = %v, want %v", tt.prev, tt.current, actual, tt.expected)
			}
		})
	}
}

func TestEmptyRootList(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()
	rootedDriver, err := s3DriverConstructor(validRoot, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := s3DriverConstructor("", s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := s3DriverConstructor("/", s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := dcontext.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	// nolint:errcheck
	defer rootedDriver.Delete(ctx, filename)

	keys, _ := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, _ = slashRootDriver.List(ctx, "/")
	for _, p := range keys {
		if !storagedriver.PathRegexp.MatchString(p) {
			t.Fatalf("unexpected string in path: %q != %q", p, storagedriver.PathRegexp)
		}
	}
}

func TestClientTransport(t *testing.T) {
	skipCheck(t)

	testCases := []struct {
		skipverify bool
	}{
		{true},
		{false},
	}

	for _, tc := range testCases {
		// NOTE(milosgajdos): we cannot simply reuse s3DriverConstructor
		// because s3DriverConstructor is initialized in init() using the process
		// env vars: we can not override S3_SKIP_VERIFY env var with t.Setenv
		params := map[string]any{
			"region":     os.Getenv("AWS_REGION"),
			"bucket":     os.Getenv("S3_BUCKET"),
			"skipverify": tc.skipverify,
		}
		t.Run(fmt.Sprintf("SkipVerify %v", tc.skipverify), func(t *testing.T) {
			drv, err := FromParameters(context.TODO(), params)
			if err != nil {
				t.Fatalf("failed to create driver: %v", err)
			}

			s3drv := drv.baseEmbed.Base.StorageDriver.(*driver)
			if tc.skipverify {
				tr, ok := s3drv.S3.Client.Config.HTTPClient.Transport.(*http.Transport)
				if !ok {
					t.Fatal("unexpected driver transport")
				}
				if !tr.TLSClientConfig.InsecureSkipVerify {
					t.Errorf("unexpected TLS Config. Expected InsecureSkipVerify: %v, got %v",
						tc.skipverify,
						tr.TLSClientConfig.InsecureSkipVerify)
				}
				// make sure the proxy is always set
				if tr.Proxy == nil {
					t.Fatal("missing HTTP transport proxy config")
				}
				return
			}
			// if tc.skipverify is false we do not override the driver
			// HTTP client transport and leave it to the AWS SDK.
			if s3drv.S3.Client.Config.HTTPClient.Transport != nil {
				t.Errorf("unexpected S3 driver client transport")
			}
		})
	}
}

func TestStorageClass(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()
	contents := []byte("contents")
	ctx := dcontext.Background()

	// We don't need to test all the storage classes, just that its selectable.
	// The first 3 are common to AWS and MinIO, so use those.
	for _, storageClass := range s3StorageClasses[:3] {
		filename := "/test-" + storageClass
		s3Driver, err := s3DriverConstructor(rootDir, storageClass)
		if err != nil {
			t.Fatalf("unexpected error creating driver with storage class %v: %v", storageClass, err)
		}

		// Can only test outposts if using s3 outposts
		if storageClass == s3.StorageClassOutposts {
			continue
		}

		err = s3Driver.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content with storage class %v: %v", storageClass, err)
		}
		// nolint:errcheck
		defer s3Driver.Delete(ctx, filename)

		driverUnwrapped := s3Driver.Base.StorageDriver.(*driver)
		resp, err := driverUnwrapped.S3.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(driverUnwrapped.Bucket),
			Key:    aws.String(driverUnwrapped.s3Path(filename)),
		})
		if err != nil {
			t.Fatalf("unexpected error retrieving file with storage class %v: %v", storageClass, err)
		}
		defer resp.Body.Close()
		// Amazon only populates this header value for non-standard storage classes
		if storageClass == noStorageClass {
			// We haven't specified a storage class so we can't confirm what it is
		} else if storageClass == s3.StorageClassStandard && resp.StorageClass != nil {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				*resp.StorageClass,
			)
		} else if storageClass != s3.StorageClassStandard && resp.StorageClass == nil {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				s3.StorageClassStandard,
			)
		} else if storageClass != s3.StorageClassStandard && storageClass != *resp.StorageClass {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				*resp.StorageClass,
			)
		}
	}
}

func TestDelete(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()

	drvr, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	type errFn func(error) bool
	type testCase struct {
		name     string
		delete   string
		expected []string
		// error validation function
		err errFn
	}

	errPathNotFound := func(err error) bool {
		if err == nil {
			return false
		}
		switch err.(type) {
		case storagedriver.PathNotFoundError:
			return true
		}
		return false
	}
	errInvalidPath := func(err error) bool {
		if err == nil {
			return false
		}
		switch err.(type) {
		case storagedriver.InvalidPathError:
			return true
		}
		return false
	}

	objs := []string{
		"/file1",
		"/file1-2",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
		"/folder1-v2/file1",
		"/folder1-v2/subfolder1/file1",
	}

	tcs := []testCase{ //nolint:prealloc
		{
			name:   "delete folder1",
			delete: "/folder1",
			expected: []string{
				"/folder1/file1",
			},
		},
		{
			name:   "delete folder2",
			delete: "/folder2",
			expected: []string{
				"/folder2/file1",
			},
		},
		{
			name:   "delete folder3",
			delete: "/folder3",
			expected: []string{
				"/folder3/file1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2/subfolder1/file1",
			},
		},
		{
			name:     "delete path that doesn't exist",
			delete:   "/path/does/not/exist",
			expected: []string{},
			err:      errPathNotFound,
		},
		{
			name:     "delete path invalid: trailing slash",
			delete:   "/path/is/invalid/",
			expected: []string{},
			err:      errInvalidPath,
		},
		{
			name:     "delete path invalid: trailing special character",
			delete:   "/path/is/invalid*",
			expected: []string{},
			err:      errInvalidPath,
		},
	}

	// create a test case for each file
	for _, p := range objs {
		tcs = append(tcs, testCase{
			name:     fmt.Sprintf("delete path:'%s'", p),
			delete:   p,
			expected: []string{p},
		})
	}

	init := func() []string {
		// init file structure matching objs
		var created []string
		for _, p := range objs {
			err := drvr.PutContent(dcontext.Background(), p, []byte("content "+p))
			if err != nil {
				fmt.Printf("unable to init file %s: %s\n", p, err)
				continue
			}
			created = append(created, p)
		}
		return created
	}

	cleanup := func(objs []string) {
		var lastErr error
		for _, p := range objs {
			err := drvr.Delete(dcontext.Background(), p)
			if err != nil {
				switch err.(type) {
				case storagedriver.PathNotFoundError:
					continue
				}
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", lastErr)
		}
	}
	defer cleanup(objs)

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			objs := init()

			err := drvr.Delete(dcontext.Background(), tc.delete)

			if tc.err != nil {
				if err == nil {
					t.Fatal("expected error")
				}
				if !tc.err(err) {
					t.Fatalf("error does not match expected: %s", err)
				}
			}
			if tc.err == nil && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			var issues []string

			// validate all files expected to be deleted are deleted
			// and all files not marked for deletion still remain
			expected := tc.expected
			isExpected := func(path string) bool {
				return slices.Contains(expected, path)
			}
			for _, path := range objs {
				stat, err := drvr.Stat(dcontext.Background(), path)
				if err != nil {
					switch err.(type) {
					case storagedriver.PathNotFoundError:
						if !isExpected(path) {
							issues = append(issues, fmt.Sprintf("unexpected path was deleted: %s", path))
						}
						// path was deleted & was supposed to be
						continue
					}
					t.Fatalf("stat: %s", err)
				}
				if stat.IsDir() {
					// for special cases where an object path has subpaths (eg /file1)
					// once /file1 is deleted it's now a directory according to stat
					continue
				}
				if isExpected(path) {
					issues = append(issues, fmt.Sprintf("expected path was not deleted: %s", path))
				}
			}

			if len(issues) > 0 {
				t.Fatal(strings.Join(issues, "; \n\t"))
			}
		})
	}
}

func TestWalkEmptyUploadsDir(t *testing.T) {
	skipCheck(t)

	ctx := dcontext.Background()

	drvr, err := s3DriverConstructor("s3walktest", s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	fileset := []string{
		"/docker/registry/v2/blobs/sha256/04/046909",
		"/docker/registry/v2/blobs/sha256/07/071a45",
		"/docker/registry/v2/repositories/testns/testimg/_layers/sha256/2a43dc",
		"/docker/registry/v2/repositories/testns/testimg/_layers/sha256/3ae7e8",
		"/docker/registry/v2/repositories/testns/testimg/_manifests/revisions/sha256/3ae7e8",
		"/docker/registry/v2/repositories/testns/testimg/_uploads/",
	}

	// create file structure matching fileset above.
	// we use the s3 sdk directly because the driver doesn't allow creation
	// of empty directories, which we need to simulate cases when purgeuploads
	// leaves behind empty directories.
	created := make([]string, 0, len(fileset))
	d := drvr.baseEmbed.Base.StorageDriver.(*driver)
	for _, p := range fileset {
		_, err := d.S3.PutObjectWithContext(ctx, &s3.PutObjectInput{
			Bucket:               aws.String(d.Bucket),
			Key:                  aws.String(d.s3Path(p)),
			ContentType:          d.getContentType(),
			ACL:                  d.getACL(),
			ServerSideEncryption: d.getEncryptionMode(),
			SSEKMSKeyId:          d.getSSEKMSKeyID(),
			StorageClass:         d.getStorageClass(),
			Body:                 bytes.NewReader([]byte("content " + p)),
		})
		if err != nil {
			fmt.Printf("unable to create file %s: %s\n", p, err)
			continue
		}
		created = append(created, p)
	}

	// use a custom cleanup here because we create an empty dir during this test's
	// setup, and the regular driver.Delete will error when trying to delete it.
	defer func() {
		s3Objects := make([]*s3.ObjectIdentifier, 0, len(fileset))
		for _, p := range created {
			s3Objects = append(s3Objects, &s3.ObjectIdentifier{
				Key: aws.String(d.s3Path(p)),
			})
		}
		resp, err := d.S3.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(d.Bucket),
			Delete: &s3.Delete{
				Objects: s3Objects,
				Quiet:   aws.Bool(false),
			},
		})
		if err != nil {
			t.Logf("DeleteObjectsWithContext resp: %+v", resp)
			t.Fatalf("cleanup failed: %s", err)
		}
	}()

	err = drvr.Walk(ctx, "/docker/registry/v2", func(fileInfo storagedriver.FileInfo) error {
		// attempt to split filepath into dir and filename, just like purgeuploads would.
		filePath := fileInfo.Path()
		_, file := path.Split(filePath)
		if len(file) == 0 {
			t.Logf("fileInfo.Path(): %s", fileInfo.Path())
			t.Fatalf("File part of fileInfo.Path() had zero length, this shouldn't happen.")
		}
		return nil
	}, func(*storagedriver.WalkOptions) {})
	if err != nil {
		t.Fatalf("driver.Walk failed: %s", err)
	}
}

func TestWalk(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()

	drvr, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	fileset := []string{
		"/file1",
		"/folder1-suffix/file1",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
	}

	// create file structure matching fileset above
	created := make([]string, 0, len(fileset))
	for _, p := range fileset {
		err := drvr.PutContent(dcontext.Background(), p, []byte("content "+p))
		if err != nil {
			fmt.Printf("unable to create file %s: %s\n", p, err)
			continue
		}
		created = append(created, p)
	}

	// cleanup
	defer func() {
		var lastErr error
		for _, p := range created {
			err := drvr.Delete(dcontext.Background(), p)
			if err != nil {
				_ = fmt.Errorf("cleanup failed for path %s: %s", p, err)
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", err)
		}
	}()

	noopFn := func(fileInfo storagedriver.FileInfo) error { return nil }

	tcs := []struct {
		name     string
		fn       storagedriver.WalkFn
		from     string
		options  []func(*storagedriver.WalkOptions)
		expected []string
		err      bool
	}{
		{
			name: "walk all",
			fn:   noopFn,
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
				"/folder3",
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "skip directory",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder3" {
					return storagedriver.ErrSkipDir
				}
				if strings.Contains(fileInfo.Path(), "/folder3") {
					t.Fatalf("skipped dir %s and should not walk %s", "/folder3", fileInfo.Path())
				}
				return nil
			},
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
				"/folder3",
				// folder 3 contents skipped
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "start late without from",
			fn:   noopFn,
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/subfolder1/subfolder1/file1"),
			},
			expected: []string{
				// start late
				"/folder3",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
				"/folder4",
				"/folder4/file1",
			},
			err: false,
		},
		{
			name: "start late with from",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/subfolder1/subfolder1/file1"),
			},
			expected: []string{
				// start late
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
			err: false,
		},
		{
			name: "start after from",
			fn:   noopFn,
			from: "/folder1",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder2"),
			},
			expected: []string{},
			err:      false,
		},
		{
			name: "start matches from",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3"),
			},
			expected: []string{
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
			err: false,
		},
		{
			name: "start doesn't exist",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/notafolder/notafile"),
			},
			expected: []string{
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
			err: false,
		},
		{
			name: "stop early",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder1/file1" {
					return storagedriver.ErrFilledBuffer
				}
				return nil
			},
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				// stop early
			},
			err: false,
		},

		{
			name: "error",
			fn: func(fileInfo storagedriver.FileInfo) error {
				return errors.New("foo")
			},
			expected: []string{
				"/file1",
			},
			err: true,
		},
		{
			name: "from folder",
			fn:   noopFn,
			expected: []string{
				"/folder1/file1",
			},
			from: "/folder1",
		},
	}

	for _, tc := range tcs {
		var walked []string
		if tc.from == "" {
			tc.from = "/"
		}
		t.Run(tc.name, func(t *testing.T) {
			err := drvr.Walk(dcontext.Background(), tc.from, func(fileInfo storagedriver.FileInfo) error {
				walked = append(walked, fileInfo.Path())
				return tc.fn(fileInfo)
			}, tc.options...)
			if tc.err && err == nil {
				t.Fatal("expected err")
			}
			if !tc.err && err != nil {
				t.Fatal(err)
			}
			compareWalked(t, tc.expected, walked)
		})
	}
}

func TestOverThousandBlobs(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()
	standardDriver, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	ctx := dcontext.Background()
	for i := range 1005 {
		filename := "/thousandfiletest/file" + strconv.Itoa(i)
		contents := []byte("contents")
		err = standardDriver.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content: %v", err)
		}
	}

	// cant actually verify deletion because read-after-delete is inconsistent, but can ensure no errors
	err = standardDriver.Delete(ctx, "/thousandfiletest")
	if err != nil {
		t.Fatalf("unexpected error deleting thousand files: %v", err)
	}
}

func TestMoveWithMultipartCopy(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()
	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := dcontext.Background()
	sourcePath := "/source"
	destPath := "/dest"

	// nolint:errcheck
	defer d.Delete(ctx, sourcePath)
	// nolint:errcheck
	defer d.Delete(ctx, destPath)

	// An object larger than d's MultipartCopyThresholdSize will cause d.Move() to perform a multipart copy.
	multipartCopyThresholdSize := d.baseEmbed.Base.StorageDriver.(*driver).MultipartCopyThresholdSize
	contents := make([]byte, 2*multipartCopyThresholdSize)
	if _, err := rand.Read(contents); err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}

	err = d.PutContent(ctx, sourcePath, contents)
	if err != nil {
		t.Fatalf("unexpected error writing content: %v", err)
	}

	err = d.Move(ctx, sourcePath, destPath)
	if err != nil {
		t.Fatalf("unexpected error moving file: %v", err)
	}

	received, err := d.GetContent(ctx, destPath)
	if err != nil {
		t.Fatalf("unexpected error getting content: %v", err)
	}
	if !bytes.Equal(contents, received) {
		t.Fatal("content differs")
	}

	_, err = d.GetContent(ctx, sourcePath)
	switch err.(type) {
	case storagedriver.PathNotFoundError:
	default:
		t.Fatalf("unexpected error getting content: %v", err)
	}
}

func TestListObjectsV2(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()
	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := dcontext.Background()
	n := 6
	prefix := "/test-list-objects-v2"
	filePaths := make([]string, 0, n)
	for i := range n {
		filePaths = append(filePaths, fmt.Sprintf("%s/%d", prefix, i))
	}
	for _, p := range filePaths {
		if err := d.PutContent(ctx, p, []byte(p)); err != nil {
			t.Fatalf("unexpected error putting content: %v", err)
		}
	}

	info, err := d.Stat(ctx, filePaths[0])
	if err != nil {
		t.Fatalf("unexpected error stating: %v", err)
	}

	if info.IsDir() || info.Size() != int64(len(filePaths[0])) || info.Path() != filePaths[0] {
		t.Fatal("unexcepted state info")
	}

	subDirPath := prefix + "/sub/0"
	if err := d.PutContent(ctx, subDirPath, []byte(subDirPath)); err != nil {
		t.Fatalf("unexpected error putting content: %v", err)
	}

	subPaths := append(filePaths, path.Dir(subDirPath))

	result, err := d.List(ctx, prefix)
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}

	sort.Strings(subPaths)
	sort.Strings(result)
	if !reflect.DeepEqual(subPaths, result) {
		t.Fatal("unexpected list result")
	}

	var walkPaths []string
	if err := d.Walk(ctx, prefix, func(fileInfo storagedriver.FileInfo) error {
		walkPaths = append(walkPaths, fileInfo.Path())
		if fileInfo.Path() == path.Dir(subDirPath) {
			if !fileInfo.IsDir() {
				t.Fatal("unexpected walking file info")
			}
		} else {
			if fileInfo.IsDir() || fileInfo.Size() != int64(len(fileInfo.Path())) {
				t.Fatal("unexpected walking file info")
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error walking: %v", err)
	}

	subPaths = append(subPaths, subDirPath)
	sort.Strings(walkPaths)
	sort.Strings(subPaths)
	if !reflect.DeepEqual(subPaths, walkPaths) {
		t.Fatal("unexpected walking paths")
	}

	if err := d.Delete(ctx, prefix); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

func compareWalked(t *testing.T, expected, walked []string) {
	if len(walked) != len(expected) {
		t.Fatalf("Mismatch number of fileInfo walked %d expected %d; walked %s; expected %s;", len(walked), len(expected), walked, expected)
	}
	for i := range walked {
		if walked[i] != expected[i] {
			t.Fatalf("walked in unexpected order: expected %s; walked %s", expected, walked)
		}
	}
}
