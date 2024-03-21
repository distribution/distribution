package token

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildAutoRedirectURL(t *testing.T) {
	cases := []struct {
		name             string
		reqGetter        func() *http.Request
		autoRedirectPath string
		expectedURL      string
	}{{
		name: "http",
		reqGetter: func() *http.Request {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			return req
		},
		autoRedirectPath: "/auth",
		expectedURL:      "https://example.com/auth",
	}, {
		name: "x-forwarded",
		reqGetter: func() *http.Request {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.Header.Set("X-Forwarded-Proto", "http")
			return req
		},
		autoRedirectPath: "/auth/token",
		expectedURL:      "http://example.com/auth/token",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.reqGetter()
			result := buildAutoRedirectURL(req, tc.autoRedirectPath)
			if result != tc.expectedURL {
				t.Errorf("expected %s, got %s", tc.expectedURL, result)
			}
		})
	}
}

func TestCheckOptions(t *testing.T) {
	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]interface{}{
		"realm":            realm,
		"issuer":           issuer,
		"service":          service,
		"rootcertbundle":   "",
		"autoredirect":     true,
		"autoredirectpath": "/auth",
	}

	ta, err := checkOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	if ta.autoRedirect != true {
		t.Fatal("autoredirect should be true")
	}
	if ta.autoRedirectPath != "/auth" {
		t.Fatal("autoredirectpath should be /auth")
	}

	options = map[string]interface{}{
		"realm":                        realm,
		"issuer":                       issuer,
		"service":                      service,
		"rootcertbundle":               "",
		"autoredirect":                 true,
		"autoredirectforcetlsdisabled": true,
	}

	ta, err = checkOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	if ta.autoRedirect != true {
		t.Fatal("autoredirect should be true")
	}
	if ta.autoRedirectPath != "/auth/token" {
		t.Fatal("autoredirectpath should be /auth/token")
	}
}
