package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
)

func TestConfigureAuthAllowsSameAuthorityRealm(t *testing.T) {
	t.Parallel()

	var serverURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="test-service"`, serverURL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)
	serverURL = upstream.URL

	tokenCreds, _, err := configureAuth("user", "pass", upstream.URL)
	if err != nil {
		t.Fatalf("configureAuth: %v", err)
	}

	realmURL, err := url.Parse(serverURL + "/token")
	if err != nil {
		t.Fatalf("parse realm: %v", err)
	}

	username, password := tokenCreds.Basic(realmURL)
	if username != "user" || password != "pass" {
		t.Fatalf("unexpected credentials for trusted realm: got (%q, %q)", username, password)
	}
}

func TestConfigureAuthRejectsLoopbackRealmOnDifferentAuthority(t *testing.T) {
	t.Parallel()

	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(evil.Close)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="test-service"`, evil.URL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)

	tokenCreds, _, err := configureAuth("user", "pass", upstream.URL)
	if err != nil {
		t.Fatalf("configureAuth: %v", err)
	}

	realmURL, err := url.Parse(evil.URL + "/token")
	if err != nil {
		t.Fatalf("parse realm: %v", err)
	}

	username, password := tokenCreds.Basic(realmURL)
	if username != "" || password != "" {
		t.Fatalf("unexpected credentials for off-origin realm: got (%q, %q)", username, password)
	}
}

func TestRealmAllowedForDockerHubStyleAuthService(t *testing.T) {
	t.Parallel()

	remoteURL, err := url.Parse("https://registry-1.docker.io")
	if err != nil {
		t.Fatalf("parse remote url: %v", err)
	}

	if !realmAllowed(remoteURL, "https://auth.docker.io/token") {
		t.Fatal("expected docker hub auth realm to remain allowed")
	}
}

func TestRealmFilteringChallengeManagerDropsOffOriginBearer(t *testing.T) {
	t.Parallel()

	remoteURL, err := url.Parse("https://registry.example.com")
	if err != nil {
		t.Fatalf("parse remote url: %v", err)
	}

	endpoint, err := url.Parse("https://registry.example.com/v2/")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     make(http.Header),
		Request:    &http.Request{URL: endpoint},
	}
	resp.Header.Add("Www-Authenticate", `Bearer realm="https://auth.example.com/token",service="registry.example.com"`)
	resp.Header.Add("Www-Authenticate", `Bearer realm="https://evil.example.net/token",service="registry.example.com"`)

	manager := challenge.NewFilteringManager(challenge.NewSimpleManager(), func(c challenge.Challenge) bool {
		return !strings.EqualFold(c.Scheme, "bearer") || realmAllowed(remoteURL, c.Parameters["realm"])
	})
	if err := manager.AddResponse(resp); err != nil {
		t.Fatalf("add response: %v", err)
	}

	challenges, err := manager.GetChallenges(*endpoint)
	if err != nil {
		t.Fatalf("get challenges: %v", err)
	}

	if len(challenges) != 1 {
		t.Fatalf("unexpected challenge count: got %d want 1", len(challenges))
	}
	if challenges[0].Scheme != "bearer" {
		t.Fatalf("unexpected surviving challenge: %+v", challenges[0])
	}
	if challenges[0].Parameters["realm"] != "https://auth.example.com/token" {
		t.Fatalf("unexpected surviving realm: %q", challenges[0].Parameters["realm"])
	}
}
