package silly

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/distribution/auth"
)

func TestSillyAccessController(t *testing.T) {
	ac := &accessController{
		realm:   "test-realm",
		service: "test-service",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ac.Authorized(r); err != nil {
			switch err := err.(type) {
			case auth.Challenge:
				err.ServeHTTP(w, r)
				return
			default:
				t.Fatalf("unexpected error authorizing request: %v", err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer resp.Body.Close()

	// Request should not be authorized
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected response status: %v != %v", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}
	req.Header.Set("Authorization", "seriously, anything")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer resp.Body.Close()

	// Request should not be authorized
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected response status: %v != %v", resp.StatusCode, http.StatusNoContent)
	}
}
