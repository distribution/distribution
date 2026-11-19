package rbac

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func hash(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func policyYAML(t *testing.T, extraUser string) string {
	y := "users:\n" +
		"  alice: {password: '" + hash(t, "apw") + "', role: user}\n" +
		"  bob: {password: '" + hash(t, "bpw") + "', role: user}\n" +
		"  root: {password: '" + hash(t, "rpw") + "', role: admin}\n" + extraUser +
		"policies:\n" +
		"  - {role: admin, namespaces: ['*'], actions: [push, pull, delete]}\n" +
		"  - {role: user, namespaces: ['${username}/*'], actions: [push, pull, delete]}\n" +
		"  - {role: user, namespaces: ['public/*'], actions: [pull]}\n"
	return y
}

func TestPolicy(t *testing.T) {
	p, err := ParsePolicy([]byte(policyYAML(t, "")))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		user, repo string
		action     Action
		want       bool
	}{
		{"alice", "alice/app", Push, true},
		{"alice", "alice/a/b/c", Delete, true},
		{"alice", "bob/app", Pull, false},
		{"alice", "alice", Pull, false},
		{"alice", "aliceevil/app", Pull, false},
		{"alice", "public/base", Pull, true},
		{"alice", "public/base", Push, false},
		{"alice", "public/base", Delete, false},
		{"root", "bob/app", Delete, true},
		{"root", "public/x", Push, true},
		{"ghost", "public/base", Pull, false},
	}
	for _, c := range cases {
		if got := p.IsAllowed(c.user, c.repo, c.action); got != c.want {
			t.Errorf("IsAllowed(%s,%s,%s)=%v want %v", c.user, c.repo, c.action, got, c.want)
		}
	}

	repos := []string{"alice/a", "bob/b", "public/p", "other/o"}
	if got, want := p.FilterRepositories("alice", repos), []string{"alice/a", "public/p"}; !reflect.DeepEqual(got, want) {
		t.Errorf("alice catalog=%v want %v", got, want)
	}
	if got := p.FilterRepositories("root", repos); !reflect.DeepEqual(got, repos) {
		t.Errorf("admin catalog=%v want %v", got, repos)
	}
	if got := p.FilterRepositories("ghost", repos); len(got) != 0 {
		t.Errorf("unknown user catalog=%v want empty", got)
	}
}

func TestAuthenticate(t *testing.T) {
	p, err := ParsePolicy([]byte(policyYAML(t, "")))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Authenticate("alice", "apw") {
		t.Error("valid credentials rejected")
	}
	if p.Authenticate("alice", "bpw") {
		t.Error("wrong password accepted")
	}
	if p.Authenticate("ghost", "apw") {
		t.Error("unknown user accepted")
	}
	if !p.IsAdmin("root") || p.IsAdmin("alice") {
		t.Error("IsAdmin wrong")
	}
}

func TestParsePolicyInvalid(t *testing.T) {
	for _, y := range []string{
		"users:\n  a: {password: '', role: user}\n",
		"policies:\n  - {role: user, namespaces: ['x/*']}\n",
		"users: [",
	} {
		if _, err := ParsePolicy([]byte(y)); err == nil {
			t.Errorf("expected error for %q", y)
		}
	}
}

func TestStoreReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rbac_policy.yml")
	if err := os.WriteFile(path, []byte(policyYAML(t, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.Get()
	if _, ok := p.Users["carol"]; ok {
		t.Fatal("carol should not exist yet")
	}

	// add a user -> picked up without restart
	y := policyYAML(t, "  carol: {password: '"+hash(t, "cpw")+"', role: user}\n")
	if err := os.WriteFile(path, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)
	p, _ = s.Get()
	if !p.Authenticate("carol", "cpw") {
		t.Error("new user not loaded")
	}

	// corrupt file -> last good policy kept
	if err := os.WriteFile(path, []byte("users: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	later := future.Add(2 * time.Second)
	_ = os.Chtimes(path, later, later)
	p, err = s.Get()
	if err != nil || !p.Authenticate("carol", "cpw") {
		t.Error("last good policy not retained")
	}
}

func TestNewStoreMissingFile(t *testing.T) {
	if _, err := NewStore(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Error("expected error")
	}
}
