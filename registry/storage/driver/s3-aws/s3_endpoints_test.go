package s3

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func isolateAWSConfig(t testing.TB) {
	t.Helper()
	t.Setenv("AWS_CONFIG_FILE", os.DevNull)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
	for _, name := range []string{
		"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_CA_BUNDLE",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3",
		"AWS_MAX_ATTEMPTS", "AWS_RETRY_MODE",
		"AWS_REQUEST_CHECKSUM_CALCULATION", "AWS_RESPONSE_CHECKSUM_VALIDATION",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, name := range []string{"AWS_USE_DUALSTACK_ENDPOINT", "AWS_USE_FIPS_ENDPOINT", "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS"} {
		t.Setenv(name, "false")
	}
}

type endpointTestHTTPClient func(*http.Request) (*http.Response, error)

func (f endpointTestHTTPClient) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Exercise the finalized request and presign pipelines, not just the resolver:
// SDK middleware can change the scheme after endpoint resolution.
func TestSDKV2EndpointSchemes(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		name, endpoint, redirect, envEndpoint, wantStorage, wantRedirect string
		secure                                                           bool
	}{
		{"explicit HTTPS overrides insecure", "https://storage.example", "", "", "https", "https", false},
		{"explicit HTTP overrides secure", "http://storage.example", "", "", "http", "http", true},
		{"schemeless HTTP", "storage.example", "", "", "http", "http", false},
		{"schemeless HTTPS", "storage.example", "", "", "https", "https", true},
		{"HTTP backend HTTPS redirect", "http://storage.example", "https://download.example", "", "http", "https", false},
		{"HTTPS backend HTTP redirect", "https://storage.example", "http://download.example", "", "https", "http", true},
		{"AWS HTTP", "", "", "", "http", "http", false},
		{"AWS HTTPS", "", "", "", "https", "https", true},
		{"AWS HTTP HTTPS redirect", "", "https://download.example", "", "http", "https", false},
		{"ambient HTTPS overrides insecure", "", "", "https://storage.example", "https", "https", false},
		{"ambient HTTP overrides secure", "", "", "http://storage.example", "http", "http", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_ENDPOINT_URL_S3", tc.envEndpoint)
			params := map[string]any{
				"accesskey": "access", "secretkey": "secret",
				"bucket": "test-bucket", "region": "us-east-1",
				"regionendpoint": tc.endpoint, "secure": tc.secure,
				"forcepathstyle": true,
			}
			if tc.redirect != "" {
				params["redirectendpoint"] = tc.redirect
			}
			d := driverFromParams(t, params)
			options := d.S3.Options()
			var requests int
			options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Scheme != tc.wantStorage {
					t.Errorf("storage scheme = %q, want %q", r.URL.Scheme, tc.wantStorage)
				}
				if r.Header.Get("Authorization") == "" {
					t.Error("storage request was not signed")
				}
				return &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{},
					Body: io.NopCloser(strings.NewReader("payload")), Request: r,
				}, nil
			})
			d.S3 = awss3.New(options)
			if _, err := d.GetContent(t.Context(), "/key"); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("storage requests = %d, want 1", requests)
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				signed, err := d.RedirectURL(httptest.NewRequest(method, "http://registry.example", nil), "/key")
				if err != nil {
					t.Fatal(err)
				}
				u, err := url.Parse(signed)
				if err != nil {
					t.Fatal(err)
				}
				if u.Scheme != tc.wantRedirect {
					t.Errorf("%s redirect scheme = %q, want %q", method, u.Scheme, tc.wantRedirect)
				}
				if tc.redirect != "" && u.Host != "download.example" {
					t.Errorf("%s redirect host = %q, want download.example", method, u.Host)
				}
				if u.Query().Get("X-Amz-Signature") == "" {
					t.Errorf("%s redirect was not signed", method)
				}
			}
		})
	}
}

func TestSDKV2EndpointPrecedence(t *testing.T) {
	for _, source := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3", "shared global", "shared service"} {
		t.Run(source, func(t *testing.T) {
			isolateAWSConfig(t)
			const ambient = "http://ambient.example"
			switch source {
			case "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3":
				t.Setenv(source, ambient)
			default:
				contents := "[default]\nendpoint_url = " + ambient + "\n"
				if source == "shared service" {
					contents = "[default]\nservices = registry-test\n[services registry-test]\ns3 =\n  endpoint_url = " + ambient + "\n"
				}
				configFile := filepath.Join(t.TempDir(), "config")
				if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_CONFIG_FILE", configFile)
			}
			for _, tc := range []struct{ name, endpoint, scheme, want string }{
				{"ambient", "", "http", "ambient.example"},
				{"explicit", "https://configured.example", "https", "configured.example"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					params := sdkTestParameters(tc.endpoint)
					params["secure"] = true
					d := driverFromParams(t, params)
					check := func(u *url.URL) {
						t.Helper()
						if u.Scheme != tc.scheme || u.Host != tc.want {
							t.Errorf("endpoint = %s://%s, want %s://%s", u.Scheme, u.Host, tc.scheme, tc.want)
						}
					}
					options := d.S3.Options()
					options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
						check(r.URL)
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("payload")), Request: r}, nil
					})
					d.S3 = awss3.New(options)
					if _, err := d.GetContent(t.Context(), "/key"); err != nil {
						t.Fatal(err)
					}
					for _, method := range []string{http.MethodGet, http.MethodHead} {
						signed, err := d.RedirectURL(httptest.NewRequest(method, "https://registry.example", nil), "/key")
						if err != nil {
							t.Fatal(err)
						}
						u, err := url.Parse(signed)
						if err != nil {
							t.Fatal(err)
						}
						check(u)
					}
				})
			}
		})
	}
}
