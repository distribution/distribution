package token

import (
	"testing"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"

	"net/http"
	"net/http/httptest"

	"github.com/go-jose/go-jose/v4"
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

func mockGetRootCerts(path string) ([]*x509.Certificate, error) {
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 1024) // not to slow down the test that much
	if err != nil {
		return nil, err
	}

	ca := &x509.Certificate{
		PublicKey: &caPrivKey.PublicKey,
	}

	return []*x509.Certificate{ca}, nil
}

func mockGetJwks(path string) (*jose.JSONWebKeySet, error) {
	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				KeyID: "sample-key-id",
			},
		},
	}, nil
}

func TestRootCertIncludedInTrustedKeys(t *testing.T) {
	old := rootCertFetcher
	rootCertFetcher = mockGetRootCerts
	defer func() { rootCertFetcher = old }()

	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]interface{}{
		"realm":            realm,
		"issuer":           issuer,
		"service":          service,
		"rootcertbundle":   "something-to-trigger-our-mock",
		"autoredirect":     true,
		"autoredirectpath": "/auth",
	}

	ac, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}
	// newAccessController return type is an interface built from
	// accessController struct. The type check can be safely ignored.
	ac2, _ := ac.(*accessController)
	if got := len(ac2.trustedKeys); got != 1 {
		t.Fatalf("Unexpected number of trusted keys, expected 1 got: %d", got)
	}
}

func TestJWKSIncludedInTrustedKeys(t *testing.T) {
	old := jwkFetcher
	jwkFetcher = mockGetJwks
	defer func() { jwkFetcher = old }()

	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]interface{}{
		"realm":            realm,
		"issuer":           issuer,
		"service":          service,
		"jwks":             "something-to-trigger-our-mock",
		"autoredirect":     true,
		"autoredirectpath": "/auth",
	}

	ac, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}
	// newAccessController return type is an interface built from
	// accessController struct. The type check can be safely ignored.
	ac2, _ := ac.(*accessController)
	if got := len(ac2.trustedKeys); got != 1 {
		t.Fatalf("Unexpected number of trusted keys, expected 1 got: %d", got)
	}
}
