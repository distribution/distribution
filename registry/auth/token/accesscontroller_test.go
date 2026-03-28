package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/sirupsen/logrus"
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

	options := map[string]any{
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

	options = map[string]any{
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

func TestCheckOptionsInvalidJWKSURL(t *testing.T) {
	base := map[string]any{
		"realm":   "https://auth.example.com/token/",
		"issuer":  "test-issuer.example.com",
		"service": "test-service.example.com",
	}

	cases := []struct {
		name string
		jwks string
	}{
		{"no host", "https://"},
		{"invalid url", "https://[::1]invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := make(map[string]any, len(base)+1)
			maps.Copy(opts, base)
			opts["jwks"] = tc.jwks

			if _, err := checkOptions(opts); err == nil {
				t.Fatalf("expected error for jwks=%q, got nil", tc.jwks)
			}
		})
	}
}

func TestCheckOptionsValidJWKSURL(t *testing.T) {
	cases := []string{
		"https://auth.example.com/.well-known/jwks.json",
		"http://localhost:8080/jwks",
	}

	base := map[string]any{
		"realm":   "https://auth.example.com/token/",
		"issuer":  "test-issuer.example.com",
		"service": "test-service.example.com",
	}

	for _, jwks := range cases {
		t.Run(jwks, func(t *testing.T) {
			opts := make(map[string]any, len(base)+1)
			maps.Copy(opts, base)
			opts["jwks"] = jwks

			ta, err := checkOptions(opts)
			if err != nil {
				t.Fatalf("unexpected error for jwks=%q: %v", jwks, err)
			}
			if ta.jwks != jwks {
				t.Fatalf("expected jwks=%q, got %q", jwks, ta.jwks)
			}
		})
	}
}

func TestRootCertIncludedInTrustedKeys(t *testing.T) {
	old := rootCertFetcher
	rootCertFetcher = mockGetRootCerts
	defer func() { rootCertFetcher = old }()

	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]any{
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

	options := map[string]any{
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
	t.Cleanup(func() { _ = ac.(*accessController).Close() })

	// newAccessController return type is an interface built from
	// accessController struct. The type check can be safely ignored.
	ac2, _ := ac.(*accessController)
	if got := len(ac2.trustedKeys); got != 1 {
		t.Fatalf("Unexpected number of trusted keys, expected 1 got: %d", got)
	}
}

func TestGetJWKSFromURL(t *testing.T) {
	// Hardcoded JWKS JSON to avoid marshaling issues with nil key material.
	const body = `{"keys":[{"kty":"oct","kid":"key-from-url","k":"c2VjcmV0"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	got, err := getJwks(srv.URL)
	if err != nil {
		t.Fatalf("getJwks from URL: %v", err)
	}
	if len(got.Keys) != 1 || got.Keys[0].KeyID != "key-from-url" {
		t.Fatalf("unexpected keys: %+v", got.Keys)
	}
}

func TestGetJWKSFromURLNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := getJwks(srv.URL); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestJWKSRefresh(t *testing.T) {
	originalLevel := logrus.GetLevel()
	t.Cleanup(func() { logrus.SetLevel(originalLevel) })
	logrus.SetLevel(logrus.FatalLevel)
	const (
		initialJWKS = `{"keys":[{"kty":"oct","kid":"initial-key","k":"c2VjcmV0"}]}`
		rotatedJWKS = `{"keys":[{"kty":"oct","kid":"rotated-key","k":"c2VjcmV0"}]}`
	)

	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body := initialJWKS
		if callCount > 1 {
			body = rotatedJWKS
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	options := map[string]any{
		"realm":               "https://auth.example.com/token/",
		"issuer":              "test-issuer.example.com",
		"service":             "test-service.example.com",
		"jwks":                srv.URL,
		"jwksrefreshinterval": "100ms",
	}

	ac, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}
	ac2 := ac.(*accessController)
	defer ac2.Close()

	// Wait for at least one refresh cycle.
	time.Sleep(200 * time.Millisecond)

	ac2.mu.RLock()
	_, hasRotated := ac2.trustedKeys["rotated-key"]
	ac2.mu.RUnlock()

	if !hasRotated {
		t.Fatalf("expected trustedKeys to contain %q after refresh, got: %v", "rotated-key", ac2.trustedKeys)
	}
}

func TestJWKSRefreshKeepsOldKeysOnError(t *testing.T) {
	originalLevel := logrus.GetLevel()
	t.Cleanup(func() { logrus.SetLevel(originalLevel) })
	logrus.SetLevel(logrus.FatalLevel)
	const initialJWKS = `{"keys":[{"kty":"oct","kid":"original-key","k":"c2VjcmV0"}]}`

	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount > 1 {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(initialJWKS))
	}))
	defer srv.Close()

	options := map[string]any{
		"realm":               "https://auth.example.com/token/",
		"issuer":              "test-issuer.example.com",
		"service":             "test-service.example.com",
		"jwks":                srv.URL,
		"jwksrefreshinterval": "100ms",
	}

	ac, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}
	ac2 := ac.(*accessController)
	defer ac2.Close()

	// Wait for a failed refresh cycle.
	time.Sleep(200 * time.Millisecond)

	ac2.mu.RLock()
	_, hasOriginal := ac2.trustedKeys["original-key"]
	ac2.mu.RUnlock()

	if !hasOriginal {
		t.Fatalf("expected original key to be preserved after failed refresh, got: %v", ac2.trustedKeys)
	}
}
