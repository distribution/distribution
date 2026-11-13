package rbac

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/registry/auth"
)

func newTestController(t *testing.T) auth.AccessController {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rbac_policy.yml")
	if err := os.WriteFile(path, []byte(policyYAML(t, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	ac, err := auth.GetAccessController("rbac", map[string]any{"realm": "test", "policy": path})
	if err != nil {
		t.Fatal(err)
	}
	return ac
}

func repoAccess(name string, actions ...string) []auth.Access {
	var out []auth.Access
	for _, a := range actions {
		out = append(out, auth.Access{Resource: auth.Resource{Type: "repository", Name: name}, Action: a})
	}
	return out
}

func request(user, pass string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	if user != "" {
		r.SetBasicAuth(user, pass)
	}
	return r
}

func TestAccessControllerOptions(t *testing.T) {
	for _, opts := range []map[string]any{
		{}, {"realm": "r"}, {"policy": "/x"}, {"realm": "r", "policy": "/does/not/exist.yml"},
	} {
		if _, err := newAccessController(opts); err == nil {
			t.Errorf("expected error for %v", opts)
		}
	}
}

func TestAuthorized(t *testing.T) {
	ac := newTestController(t)
	cases := []struct {
		name       string
		user, pass string
		access     []auth.Access
		wantOK     bool
	}{
		{"ping with valid creds", "alice", "apw", nil, true},
		{"own pull+push (what POST/PUT/PATCH request)", "alice", "apw", repoAccess("alice/app", "pull", "push"), true},
		{"own delete", "alice", "apw", repoAccess("alice/app", "delete"), true},
		{"other user's pull", "alice", "apw", repoAccess("bob/app", "pull"), false},
		{"other user's push", "alice", "apw", repoAccess("bob/app", "pull", "push"), false},
		{"other user's delete", "alice", "apw", repoAccess("bob/app", "delete"), false},
		{"public pull", "alice", "apw", repoAccess("public/base", "pull"), true},
		{"public push", "alice", "apw", repoAccess("public/base", "pull", "push"), false},
		{"public delete", "alice", "apw", repoAccess("public/base", "delete"), false},
		{"admin anywhere", "root", "rpw", repoAccess("bob/app", "pull", "push"), true},
		{"admin public push", "root", "rpw", repoAccess("public/base", "pull", "push"), true},
		{"blob mount: own target + foreign source", "alice", "apw",
			append(repoAccess("alice/app", "pull", "push"), repoAccess("bob/app", "pull")...), false},
		{"blob mount: own target + public source", "alice", "apw",
			append(repoAccess("alice/app", "pull", "push"), repoAccess("public/base", "pull")...), true},
		{"catalog", "alice", "apw", []auth.Access{{Resource: auth.Resource{Type: "registry", Name: "catalog"}, Action: "*"}}, true},
		{"unknown resource type", "alice", "apw", []auth.Access{{Resource: auth.Resource{Type: "other", Name: "x"}, Action: "pull"}}, false},
		{"wrong password", "alice", "nope", nil, false},
		{"unknown user", "ghost", "x", nil, false},
		{"no credentials", "", "", nil, false},
	}
	for _, c := range cases {
		grant, err := ac.Authorized(request(c.user, c.pass), c.access...)
		if c.wantOK {
			if err != nil || grant == nil || grant.User.Name != c.user {
				t.Errorf("%s: want grant for %q, got grant=%v err=%v", c.name, c.user, grant, err)
			}
			continue
		}
		if err == nil || grant != nil {
			t.Errorf("%s: expected denial", c.name)
			continue
		}
		// Denials must be challenges: the registry maps any other error type to a bare 400.
		ch, ok := err.(auth.Challenge)
		if !ok {
			t.Errorf("%s: error %T is not an auth.Challenge", c.name, err)
			continue
		}
		w := httptest.NewRecorder()
		ch.SetHeaders(request("", ""), w)
		if w.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: missing WWW-Authenticate", c.name)
		}
	}
}

func TestAuthorizedKeepsLastGoodPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rbac_policy.yml")
	if err := os.WriteFile(path, []byte(policyYAML(t, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	ac, err := newAccessController(map[string]any{"realm": "r", "policy": path})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	if _, err := ac.Authorized(request("alice", "apw"), repoAccess("alice/x", "pull")...); err != nil {
		t.Errorf("last good policy should keep working: %v", err)
	}
	if _, err := ac.Authorized(request("alice", "apw"), repoAccess("bob/x", "pull")...); err == nil {
		t.Error("must still deny cross-namespace access")
	}
}

func TestSharedStoreIsShared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rbac_policy.yml")
	if err := os.WriteFile(path, []byte(policyYAML(t, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := SharedStore(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := SharedStore(path)
	if a != b {
		t.Error("SharedStore must return the same Store for the same path")
	}
}
