package s3

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// SDK client construction mutates its Express credentials provider, even for
// ordinary buckets. RedirectURL must not reconstruct clients during requests.
func TestSDKV2ConcurrentRedirects(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		redirect  string
		pathStyle bool
	}{{"", true}, {"https://download.example", true}, {"", false}, {"https://download.example", false}} {
		t.Run(fmt.Sprintf("%s/pathstyle=%t", tc.redirect, tc.pathStyle), func(t *testing.T) {
			params := sdkTestParameters("http://storage.example")
			params["redirectendpoint"] = tc.redirect
			params["forcepathstyle"] = tc.pathStyle
			backendHost := "storage.example"
			if !tc.pathStyle {
				backendHost = "test-bucket." + backendHost
			}
			d := driverFromParams(t, params)
			options := d.S3.Options()
			options.HTTPClient = endpointTestHTTPClient(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "http" || r.URL.Host != backendHost {
					t.Errorf("backend endpoint changed: %s://%s", r.URL.Scheme, r.URL.Host)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("payload")), Request: r}, nil
			})
			d.S3 = awss3.New(options)
			wantScheme, wantHost := "http", "storage.example"
			if tc.redirect != "" {
				wantScheme, wantHost = "https", "download.example"
			}
			if !tc.pathStyle {
				wantHost = "test-bucket." + wantHost
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			for range 8 {
				wg.Go(func() {
					<-start
					for range 32 {
						if _, err := d.GetContent(t.Context(), "/key"); err != nil {
							t.Error(err)
							return
						}
						for _, method := range []string{http.MethodGet, http.MethodHead} {
							signed, err := d.RedirectURL(httptest.NewRequest(method, "https://registry.example", nil), "/key")
							if err != nil {
								t.Error(err)
								return
							}
							u, err := url.Parse(signed)
							if err != nil {
								t.Error(err)
								return
							}
							if u.Scheme != wantScheme || u.Host != wantHost {
								t.Errorf("redirect endpoint = %s://%s, want %s://%s", u.Scheme, u.Host, wantScheme, wantHost)
							}
							if u.Query().Get("X-Amz-Signature") == "" {
								t.Error("redirect was not signed")
							}
						}
					}
				})
			}
			close(start)
			wg.Wait()
		})
	}
}
