package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/docker/distribution/testutil"
)

func assertEqualStrings(t *testing.T, name, expected, actual string) {
	if expected != actual {
		t.Errorf("Unexpected %s value %q, expected %q", name, actual, expected)
	}
}

func assertEqualConfigs(t *testing.T, expected, actual OAuth2Config) {
	assertEqualStrings(t, "ClientID", expected.ClientID, actual.ClientID)
	assertEqualStrings(t, "AuthURL", expected.AuthURL, actual.AuthURL)
	assertEqualStrings(t, "CallbackURL", expected.CallbackURL, actual.CallbackURL)
	assertEqualStrings(t, "CodeURL", expected.CodeURL, actual.CodeURL)
	assertEqualStrings(t, "LandingURL", expected.LandingURL, actual.LandingURL)
	if len(expected.Scopes) != len(actual.Scopes) {
		t.Errorf("Unexpected number of scopes %d, expected %d", len(actual.Scopes), len(expected.Scopes))
	} else {
		sort.Strings(expected.Scopes)
		sort.Strings(actual.Scopes)
		for i := range expected.Scopes {
			if expected.Scopes[i] != actual.Scopes[i] {
				t.Errorf("Unexpected scope %q, expected %q", actual.Scopes[i], expected.Scopes[i])
				break
			}
		}
	}
}

func TestGetHeaderConfig(t *testing.T) {
	testValues := []struct {
		Header   string
		Expected OAuth2Config
		Error    bool
	}{
		{
			Header: `OAuth2 client_id="5",auth_url="http://localhost:8080/authorize",callback_url="http://localhost:8080/oauth2callback",code_url="http://localhost:8081/oauth2code",landing_url="https://docs.docker.com/registry/spec",scopes="fun fun fun"`,
			Expected: OAuth2Config{
				ClientID:    "5",
				AuthURL:     "http://localhost:8080/authorize",
				CallbackURL: "http://localhost:8080/oauth2callback",
				CodeURL:     "http://localhost:8081/oauth2code",
				LandingURL:  "https://docs.docker.com/registry/spec",
				Scopes:      []string{"fun", "fun", "fun"},
			},
			Error: false,
		},
		{
			Header: `OAuth2 client_id="",auth_url="http://localhost:8080/authorize",callback_url="http://localhost:8080/oauth2callback",landing_url="https://docs.docker.com/registry/spec"`,
			Expected: OAuth2Config{
				AuthURL:     "http://localhost:8080/authorize",
				CallbackURL: "http://localhost:8080/oauth2callback",
				LandingURL:  "https://docs.docker.com/registry/spec",
				Scopes:      []string{},
			},
			Error: false,
		},
		{
			Header: `OAuth2 client_id="",auth_url="",scopes="onescope"`,
			Expected: OAuth2Config{
				Scopes: []string{"onescope"},
			},
			Error: false,
		},
		{
			Header:   `OAuth2 client_id="",auth_url=""`,
			Expected: OAuth2Config{},
			Error:    false,
		},
		{
			Header:   `NotOAuth2 client_id="",auth_url=""`,
			Expected: OAuth2Config{},
			Error:    true,
		},
	}

	var m []testutil.RequestResponseMapping
	for i := range testValues {
		m = append(m, testutil.RequestResponseMapping{
			Request: testutil.Request{
				Method: "HEAD",
				Route:  "/v2/token",
			},
			Response: testutil.Response{
				StatusCode: http.StatusUnauthorized,
				Headers: http.Header{
					"WWW-Authenticate": {testValues[i].Header},
				},
			},
		})
	}
	ts := httptest.NewServer(testutil.NewHandler(m))
	defer ts.Close()

	challenge := Challenge{
		Scheme: "bearer",
		Parameters: map[string]string{
			"realm": ts.URL + "/v2/token",
		},
	}

	for i := range testValues {
		config, err := GetOAuth2Config(http.DefaultClient, challenge)
		if err != nil {
			if !testValues[i].Error {
				t.Error(err)
			}
			continue
		}
		assertEqualConfigs(t, testValues[i].Expected, config)

		header := config.HeaderValue()
		if header != testValues[i].Header {
			t.Errorf("Unexpected header value\n%s, expected\n%s", header, testValues[i].Header)
		}
	}
}

func TestCallbackHandler(t *testing.T) {
	state := "93282342"
	callbackURL := "http://localhost:12945/oauth2callback"

	handler, err := NewOAuth2CallbackHandler(callbackURL, state, "https://docs.docker.com/registry/spec/auth/oauth/")
	if err != nil {
		t.Fatal(err)
	}

	redirectErr := errors.New("skip redirect")
	client := http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return redirectErr
		},
	}

	expectedCode := "329118352"

	resp, err := client.Get(fmt.Sprintf("%s?state=%s&code=%s", callbackURL, state, expectedCode))
	if err != nil {
		if urlErr, ok := err.(*url.Error); !ok || urlErr.Err != redirectErr {
			t.Fatal(err)
		}
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("Unexpected error code %d, expected %d", resp.StatusCode, http.StatusMovedPermanently)
	}
	location, err := resp.Location()
	if err != nil {
		t.Fatal(err)
	}
	if expected := "docs.docker.com"; location.Host != expected {
		t.Fatalf("Unexpected redirect host %s, expected %s", location.Host, expected)
	}
	if expected := "/registry/spec/auth/oauth/"; location.Path != expected {
		t.Fatalf("Unexpected redirect path %s, expected %s", location.Path, expected)
	}

	code := <-handler.CodeChan()

	if err := handler.Error(); err != nil {
		t.Fatal(err)
	}

	if code != expectedCode {
		t.Fatalf("Unexpected code %q, expected %q", code, expectedCode)
	}
}

func TestCancelHandler(t *testing.T) {
	state := "93282342"
	callbackURL := "http://localhost:12946/oauth2callback"

	handler, err := NewOAuth2CallbackHandler(callbackURL, state, "https://docs.docker.com/registry/spec/auth/oauth/")
	if err != nil {
		t.Fatal(err)
	}

	cancelErr := errors.New("OAuth2 handler is cancelled")

	handler.Cancel(cancelErr)

	code := <-handler.CodeChan()

	if err := handler.Error(); err == nil {
		t.Fatal("Expected error after cancel")
	} else if err != cancelErr {
		t.Fatalf("Unexpected error %s", err)
	}

	if code != "" {
		t.Fatalf("Unexpected empty code, got %s", code)
	}
}

type codeServer struct {
	code  string
	state string
}

func (c codeServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if values.Get("state") != c.state {
		http.Error(rw, "unknown state", http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(rw, "PING\n")
		time.Sleep(2 * time.Millisecond)
	}
	fmt.Fprintf(rw, "CODE %s\n", c.code)
}

func TestPollHandler(t *testing.T) {
	cs := codeServer{
		code:  "18371413",
		state: "938723",
	}

	ts := httptest.NewServer(cs)
	defer ts.Close()

	handler, err := NewOAuth2PollHandler(http.DefaultClient, ts.URL+"/v2/oauth2code", cs.state)
	if err != nil {
		t.Fatal(err)
	}

	code := <-handler.CodeChan()

	if err := handler.Error(); err != nil {
		t.Fatal(err)
	}

	if code != cs.code {
		t.Fatalf("Unexpected code %q, expected %q", code, cs.code)
	}
}

func TestCancelPollHandler(t *testing.T) {
	cs := codeServer{
		code:  "18371413",
		state: "938723",
	}

	ts := httptest.NewServer(cs)
	defer ts.Close()

	handler, err := NewOAuth2PollHandler(http.DefaultClient, ts.URL+"/v2/oauth2code", cs.state)
	if err != nil {
		t.Fatal(err)
	}

	cancelErr := errors.New("OAuth2 handler is cancelled")

	handler.Cancel(cancelErr)

	code := <-handler.CodeChan()

	if err := handler.Error(); err == nil {
		t.Fatal("Expected error after cancel")
	} else if err != cancelErr {
		t.Fatalf("Unexpected error %s", err)
	}

	if code != "" {
		t.Fatalf("Unexpected empty code, got %s", code)
	}
}
