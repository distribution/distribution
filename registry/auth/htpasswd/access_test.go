package htpasswd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/registry/auth"
)

func TestBasicAccessController(t *testing.T) {
	testRealm := "The-Shire"
	testUsers := []string{"bilbo", "frodo", "MiShil", "DeokMan", "nonexistent"}
	testPasswords := []string{"baggins", "baggins", "새주", "공주님", "nonexistent"}
	testHtpasswdContent := `bilbo:{SHA}5siv5c0SHx681xU6GiSx9ZQryqs=
							frodo:$2y$05$926C3y10Quzn/LnqQH86VOEVh/18T6RnLaS.khre96jLNL/7e.K5W
							MiShil:$2y$05$0oHgwMehvoe8iAWS8I.7l.KoECXrwVaC16RPfaSCU5eVTFrATuMI2
							DeokMan:공주님`

	tempFile := filepath.Join(t.TempDir(), "htpasswd")
	err := os.WriteFile(tempFile, []byte(testHtpasswdContent), 0600)
	if err != nil {
		t.Fatal("could not write temporary htpasswd file")
	}

	accessCtrl, err := newAccessController(map[string]any{
		"realm": testRealm,
		"path":  tempFile,

		"overrideDummyHash": []byte("$2a$05$/vyFmJBPzsrsp6EC53biLulrw8zVjsWqpw26Hb.wfMyrHmRdh2orW"), // hash of "nonexistent"
	})
	if err != nil {
		t.Fatal("error creating access controller")
	}

	userNumber := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant, err := accessCtrl.Authorized(r)
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
			t.Fatal("basic accessController did not return auth grant")
		}

		if grant.User.Name != testUsers[userNumber] {
			t.Fatalf("expected user name %q, got %q", testUsers[userNumber], grant.User.Name)
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	client := &http.Client{
		CheckRedirect: nil,
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer resp.Body.Close()

	// Request should not be authorized
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected non-fail response status: %v != %v", resp.StatusCode, http.StatusUnauthorized)
	}

	nonbcrypt := map[string]struct{}{
		"bilbo":       {},
		"DeokMan":     {},
		"nonexistent": {},
	}

	for i := range testUsers {
		userNumber = i
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("error allocating new request: %v", err)
		}

		req.SetBasicAuth(testUsers[i], testPasswords[i])

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("unexpected error during GET: %v", err)
		}
		defer resp.Body.Close()

		if _, ok := nonbcrypt[testUsers[i]]; ok {
			// these are not allowed.
			// Request should be authorized
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("unexpected non-success response status: %v != %v for %s %s", resp.StatusCode, http.StatusUnauthorized, testUsers[i], testPasswords[i])
			}
		} else {
			// Request should be authorized
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("unexpected non-success response status: %v != %v for %s %s", resp.StatusCode, http.StatusNoContent, testUsers[i], testPasswords[i])
			}
		}
	}

	for i := range len(testUsers) {
		userNumber = i
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("error allocating new request: %v", err)
		}

		invalidPassword := testPasswords[i] + "invalid"
		req.SetBasicAuth(testUsers[i], invalidPassword)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("unexpected error during GET: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unexpected non-success response status: %v != %v for %s %s", resp.StatusCode, http.StatusUnauthorized, testUsers[i], invalidPassword)
		}
	}
}

func TestCreateHtpasswdFile(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "htpasswd")
	if err := os.WriteFile(tempFile, []byte{}, 0600); err != nil {
		t.Fatalf("could not create temporary htpasswd file %v", err)
	}
	options := map[string]any{
		"realm": "/auth/htpasswd",
		"path":  tempFile,
	}
	// Ensure file is not populated
	if _, err := newAccessController(options); err != nil {
		t.Fatalf("error creating access controller %v", err)
	}
	if content, err := os.ReadFile(tempFile); err != nil {
		t.Fatalf("failed to read file %v", err)
	} else if !bytes.Equal([]byte{}, content) {
		t.Fatalf("htpasswd file should not be populated %v", string(content))
	}
	if err := os.Remove(tempFile); err != nil {
		t.Fatalf("failed to remove temp file %v", err)
	}

	// Ensure htpasswd file is populated
	if _, err := newAccessController(options); err != nil {
		t.Fatalf("error creating access controller %v", err)
	}
	if content, err := os.ReadFile(tempFile); err != nil {
		t.Fatalf("failed to read file %v", err)
	} else if !bytes.HasPrefix(content, []byte("docker:$2a$")) {
		t.Fatalf("failed to find default user in file %s", string(content))
	}
}
