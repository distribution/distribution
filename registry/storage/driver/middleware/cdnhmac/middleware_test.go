package middleware

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/stretchr/testify/require"
)

type mockSD struct {
	base.Base
	redirect string
	err      error
}

func (m *mockSD) RedirectURL(_ *http.Request, _ string) (string, error) {
	return m.redirect, m.err
}

const (
	testSecret = "mysecret"

	presignedQuery = "X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIA%2F20260921%2Fauto%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20260921T000000Z&X-Amz-Expires=1200&X-Amz-SignedHeaders=host&X-Amz-Signature=deadbeef"

	blobPath     = "/blobs/sha256/ab/abcd/data"
	presignedURL = "https://cdn.example.com" + blobPath + "?" + presignedQuery
)

var testTime = time.Unix(1789574400, 0)

func writeFile(t *testing.T, content string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

	return p
}

func TestNewCDNHMACStorageMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		options    func(t *testing.T) map[string]any
		wantSecret string
		wantParam  string
		wantErr    string
	}{
		{
			name:       "inline secret",
			options:    func(*testing.T) map[string]any { return map[string]any{"secret": testSecret} },
			wantSecret: testSecret,
			wantParam:  defaultParam,
		},
		{
			name: "secretfile with surrounding whitespace",
			options: func(t *testing.T) map[string]any {
				return map[string]any{"secretfile": writeFile(t, " "+testSecret+"\n")}
			},
			wantSecret: testSecret,
			wantParam:  defaultParam,
		},
		{
			name:       "custom param",
			options:    func(*testing.T) map[string]any { return map[string]any{"secret": testSecret, "param": "tok"} },
			wantSecret: testSecret,
			wantParam:  "tok",
		},
		{
			name:    "no secret",
			options: func(*testing.T) map[string]any { return map[string]any{} },
			wantErr: errNoSecret.Error(),
		},
		{
			name: "secret and secretfile",
			options: func(t *testing.T) map[string]any {
				return map[string]any{"secret": testSecret, "secretfile": writeFile(t, testSecret)}
			},
			wantErr: errSecretConflict.Error(),
		},
		{
			name: "missing secretfile",
			options: func(t *testing.T) map[string]any {
				return map[string]any{"secretfile": filepath.Join(t.TempDir(), "nope")}
			},
			wantErr: "failed to read secretfile",
		},
		{
			name:    "empty secretfile",
			options: func(t *testing.T) map[string]any { return map[string]any{"secretfile": writeFile(t, " \n")} },
			wantErr: "is empty",
		},
		{
			name:    "secret of wrong type",
			options: func(*testing.T) map[string]any { return map[string]any{"secret": 1} },
			wantErr: "secret must be a string",
		},
		{
			name:    "param of wrong type",
			options: func(*testing.T) map[string]any { return map[string]any{"secret": testSecret, "param": 1} },
			wantErr: "param must be a string",
		},
		{
			name:    "param needing escaping",
			options: func(*testing.T) map[string]any { return map[string]any{"secret": testSecret, "param": "a&b"} },
			wantErr: errInvalidParam.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mw, err := newCDNHMACStorageMiddleware(t.Context(), &mockSD{}, tt.options(t))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			m, ok := mw.(*cdnHMACStorageMiddleware)
			require.True(t, ok)
			require.Equal(t, []byte(tt.wantSecret), m.secret)
			require.Equal(t, tt.wantParam, m.param)
		})
	}
}

// The expected tokens are the Cloudflare format with flags 's' (URL-safe alphabet, no
// padding), computed independently:
//
//	python3 -c 'import hmac,hashlib,base64
//	print(base64.urlsafe_b64encode(hmac.new(b"mysecret", b"/blobs/sha256/ab/abcd/data1789574400", hashlib.sha256).digest()).decode().rstrip("="))'
//
func TestTokenFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		message  string
		expected string
	}{
		{message: blobPath, expected: "1789574400-ZFO9FKzVDoCKQBKXR7f1NfyslrvHc_hOW0uQ4oZBrzE"},
		{message: "/assets/abc123", expected: "1789574400-eC5Up-woFk7j0Dk-tz8TPgisTTEmjouO1v1M-jj-WrY"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			t.Parallel()

			got := token([]byte(testSecret), tt.message, testTime)
			require.Equal(t, tt.expected, got)
			// the token is emitted unescaped, so it must be query-string safe.
			require.Equal(t, got, url.QueryEscape(got))
		})
	}
}

// The token covers the whole URI ahead of it, including the presigned query string, and
// must be the last parameter. The existing query must survive byte for byte: re-encoding
// it would invalidate the SigV4 signature and change the message the WAF validates.
func TestRedirectURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		redirect string
		options  map[string]any
		expected string
	}{
		{
			name:     "empty query",
			redirect: "https://cdn.example.com" + blobPath,
			options:  map[string]any{"secret": testSecret},
			expected: "https://cdn.example.com" + blobPath + "?verify=" + token([]byte(testSecret), blobPath, testTime),
		},
		{
			name:     "presigned query preserved verbatim",
			redirect: presignedURL,
			options:  map[string]any{"secret": testSecret},
			expected: presignedURL + "&verify=" + token([]byte(testSecret), blobPath+"?"+presignedQuery, testTime),
		},
		{
			name:     "custom param",
			redirect: presignedURL,
			options:  map[string]any{"secret": testSecret, "param": "tok"},
			expected: presignedURL + "&tok=" + token([]byte(testSecret), blobPath+"?"+presignedQuery, testTime),
		},
		{
			// the driver declined to presign, so the registry falls back to streaming.
			name:     "empty redirect passed through",
			redirect: "",
			options:  map[string]any{"secret": testSecret},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mw, err := newCDNHMACStorageMiddleware(t.Context(), &mockSD{redirect: tt.redirect}, tt.options)
			require.NoError(t, err)

			m, ok := mw.(*cdnHMACStorageMiddleware)
			require.True(t, ok)
			m.now = func() time.Time { return testTime }

			got, err := m.RedirectURL(nil, "")
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestRedirectURLDriverError(t *testing.T) {
	t.Parallel()

	driverErr := errors.New("boom")

	mw, err := newCDNHMACStorageMiddleware(t.Context(), &mockSD{redirect: presignedURL, err: driverErr}, map[string]any{"secret": testSecret})
	require.NoError(t, err)

	got, err := mw.RedirectURL(nil, "")
	require.ErrorIs(t, err, driverErr)
	require.Empty(t, got)
}
