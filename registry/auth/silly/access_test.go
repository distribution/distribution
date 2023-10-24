package silly

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/distribution/distribution/v3/registry/auth"
)

func TestSillyAccessController(t *testing.T) {
	ac := &accessController{
		realm:   "test-realm",
		service: "test-service",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant, err := ac.Authorized(r)
		if err != nil {
			switch err := err.(type) {
			case auth.Challenge:
				err.SetHeaders(r, w)
				w.WriteHeader(http.StatusUnauthorized)
				return
			default:
				t.Fatalf("unexpected error authorizing request: %v", err)
			}
		}

		if grant == nil {
			t.Fatal("silly accessController did not return auth grant")
		}

		if grant.User.Name != "silly" {
			t.Fatalf("expected user name %q, got %q", "silly", grant.User.Name)
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

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
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
