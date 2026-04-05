package challenge

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestAuthChallengeParse(t *testing.T) {
	header := http.Header{}
	header.Add("WWW-Authenticate", `Bearer realm="https://auth.example.com/token",service="registry.example.com",other=fun,slashed="he\"\l\lo"`)

	challenges := parseAuthHeader(header)
	if len(challenges) != 1 {
		t.Fatalf("Unexpected number of auth challenges: %d, expected 1", len(challenges))
	}
	challenge := challenges[0]

	if expected := "bearer"; challenge.Scheme != expected {
		t.Fatalf("Unexpected scheme: %s, expected: %s", challenge.Scheme, expected)
	}

	if expected := "https://auth.example.com/token"; challenge.Parameters["realm"] != expected {
		t.Fatalf("Unexpected param: %s, expected: %s", challenge.Parameters["realm"], expected)
	}

	if expected := "registry.example.com"; challenge.Parameters["service"] != expected {
		t.Fatalf("Unexpected param: %s, expected: %s", challenge.Parameters["service"], expected)
	}

	if expected := "fun"; challenge.Parameters["other"] != expected {
		t.Fatalf("Unexpected param: %s, expected: %s", challenge.Parameters["other"], expected)
	}

	if expected := "he\"llo"; challenge.Parameters["slashed"] != expected {
		t.Fatalf("Unexpected param: %s, expected: %s", challenge.Parameters["slashed"], expected)
	}
}

func TestAuthChallengeNormalization(t *testing.T) {
	testAuthChallengeNormalization(t, "reg.EXAMPLE.com")
	testAuthChallengeNormalization(t, "bɿɒʜɔiɿ-ɿɘƚƨim-ƚol-ɒ-ƨʞnɒʜƚ.com")
	testAuthChallengeNormalization(t, "reg.example.com:80")
	testAuthChallengeConcurrent(t, "reg.EXAMPLE.com")
}

func testAuthChallengeNormalization(t *testing.T, host string) {
	scm := NewSimpleManager()

	url, err := url.Parse(fmt.Sprintf("http://%s/v2/", host))
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		Request: &http.Request{
			URL: url,
		},
		Header:     make(http.Header),
		StatusCode: http.StatusUnauthorized,
	}
	resp.Header.Add("WWW-Authenticate", fmt.Sprintf("Bearer realm=\"https://%s/token\",service=\"registry.example.com\"", host))

	err = scm.AddResponse(resp)
	if err != nil {
		t.Fatal(err)
	}

	lowered := *url
	lowered.Host = strings.ToLower(lowered.Host)
	lowered.Host = canonicalAddr(&lowered)
	c, err := scm.GetChallenges(lowered)
	if err != nil {
		t.Fatal(err)
	}

	if len(c) == 0 {
		t.Fatal("Expected challenge for lower-cased-host URL")
	}
}

func testAuthChallengeConcurrent(t *testing.T, host string) {
	scm := NewSimpleManager()

	url, err := url.Parse(fmt.Sprintf("http://%s/v2/", host))
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		Request: &http.Request{
			URL: url,
		},
		Header:     make(http.Header),
		StatusCode: http.StatusUnauthorized,
	}
	resp.Header.Add("WWW-Authenticate", fmt.Sprintf("Bearer realm=\"https://%s/token\",service=\"registry.example.com\"", host))
	var s sync.WaitGroup
	s.Add(2)
	go func() {
		defer s.Done()
		for range 200 {
			err = scm.AddResponse(resp)
			if err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer s.Done()
		lowered := *url
		lowered.Host = strings.ToLower(lowered.Host)
		for range 200 {
			_, err := scm.GetChallenges(lowered)
			if err != nil {
				t.Error(err)
			}
		}
	}()
	s.Wait()
}

func TestFilteringManager(t *testing.T) {
	t.Parallel()

	base := NewSimpleManager()
	manager := NewFilteringManager(base, func(c Challenge) bool {
		return c.Parameters["service"] == "keep.example.com"
	})

	endpoint, err := url.Parse("https://registry.example.com/v2/")
	if err != nil {
		t.Fatal(err)
	}

	resp := &http.Response{
		Request: &http.Request{
			URL: endpoint,
		},
		Header:     make(http.Header),
		StatusCode: http.StatusUnauthorized,
	}
	resp.Header.Add("WWW-Authenticate", `Bearer realm="https://auth.example.com/token",service="keep.example.com"`)
	resp.Header.Add("WWW-Authenticate", `Bearer realm="https://evil.example.net/token",service="drop.example.net"`)

	if err := manager.AddResponse(resp); err != nil {
		t.Fatal(err)
	}

	challenges, err := manager.GetChallenges(*endpoint)
	if err != nil {
		t.Fatal(err)
	}

	if len(challenges) != 1 {
		t.Fatalf("unexpected challenge count: got %d want 1", len(challenges))
	}

	if got := challenges[0].Parameters["service"]; got != "keep.example.com" {
		t.Fatalf("unexpected surviving challenge service: %q", got)
	}
}
