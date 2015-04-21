package basic

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/distribution/registry/auth"
	"golang.org/x/net/context"
)

func TestBasicAccessController(t *testing.T) {

	testRealm := "The-Shire"
	testUser := "bilbo"
	testHtpasswdContent := "bilbo:{SHA}5siv5c0SHx681xU6GiSx9ZQryqs="

	tempFile, err := ioutil.TempFile("", "htpasswd-test")
	if err != nil {
		t.Fatal("could not create temporary htpasswd file")
	}
	if _, err = tempFile.WriteString(testHtpasswdContent); err != nil {
		t.Fatal("could not write temporary htpasswd file")
	}

	options := map[string]interface{}{
		"realm": testRealm,
		"path":  tempFile.Name(),
	}

	accessController, err := newAccessController(options)
	if err != nil {
		t.Fatal("error creating access controller")
	}

	tempFile.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(nil, "http.request", r)
		authCtx, err := accessController.Authorized(ctx)
		if err != nil {
			switch err := err.(type) {
			case auth.Challenge:
				err.ServeHTTP(w, r)
				return
			default:
				t.Fatalf("unexpected error authorizing request: %v", err)
			}
		}

		userInfo, ok := authCtx.Value("auth.user").(auth.UserInfo)
		if !ok {
			t.Fatal("basic accessController did not set auth.user context")
		}

		if userInfo.Name != testUser {
			t.Fatalf("expected user name %q, got %q", testUser, userInfo.Name)
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	client := &http.Client{
		CheckRedirect: nil,
	}

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)

	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer resp.Body.Close()

	// Request should not be authorized
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected non-fail response status: %v != %v", resp.StatusCode, http.StatusUnauthorized)
	}

	req, _ = http.NewRequest("GET", server.URL, nil)

	sekrit := "bilbo:baggins"
	credential := "Basic " + base64.StdEncoding.EncodeToString([]byte(sekrit))

	req.Header.Set("Authorization", credential)
	resp, err = client.Do(req)

	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer resp.Body.Close()

	// Request should be authorized
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected non-success response status: %v != %v", resp.StatusCode, http.StatusNoContent)
	}

}
