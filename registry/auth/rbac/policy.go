// Package rbac implements a file-backed users + role policy engine shared by
// the RBAC reverse proxy and (later) an in-registry auth backend.
//
// Policy file format (rbac_policy.yml):
//
//	users:
//	  alice: {password: "<bcrypt hash>", role: user}
//	  root:  {password: "<bcrypt hash>", role: admin}
//	policies:
//	  - {role: admin, namespaces: ["*"],               actions: [push, pull, delete]}
//	  - {role: user,  namespaces: ["${username}/*"],   actions: [push, pull, delete]}
//	  - {role: user,  namespaces: ["public/*"],        actions: [pull]}
package rbac

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v2"
	"golang.org/x/crypto/bcrypt"
)

// Action is an operation on a repository.
type Action string

// Supported actions.
const (
	Pull   Action = "pull"
	Push   Action = "push"
	Delete Action = "delete"
)

const usernameVar = "${username}"

// dummyHash is compared against when a user does not exist so that unknown
// users cost the same as a wrong password (timing side channel guard).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("rbac-dummy"), bcrypt.DefaultCost)

// User is an entry of the users map.
type User struct {
	Password string `yaml:"password"`
	Role     string `yaml:"role"`
}

// Rule grants actions on namespaces to a role.
type Rule struct {
	Role       string   `yaml:"role"`
	Namespaces []string `yaml:"namespaces"`
	Actions    []string `yaml:"actions"`
}

// Policy is the parsed policy file.
type Policy struct {
	Users    map[string]User `yaml:"users"`
	Policies []Rule          `yaml:"policies"`
}

// ParsePolicy parses and validates policy YAML.
func ParsePolicy(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("rbac: parse policy: %w", err)
	}
	for name, u := range p.Users {
		if name == "" || u.Password == "" || u.Role == "" {
			return nil, fmt.Errorf("rbac: user %q requires password and role", name)
		}
	}
	for i, r := range p.Policies {
		if r.Role == "" || len(r.Namespaces) == 0 || len(r.Actions) == 0 {
			return nil, fmt.Errorf("rbac: policy #%d requires role, namespaces and actions", i)
		}
	}
	return &p, nil
}

// Authenticate verifies a username/password pair.
func (p *Policy) Authenticate(username, password string) bool {
	u, ok := p.Users[username]
	hash := []byte(u.Password)
	if !ok {
		hash = dummyHash
	}
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	return ok && err == nil
}

// IsAdmin reports whether the user has the admin role.
func (p *Policy) IsAdmin(username string) bool {
	u, ok := p.Users[username]
	return ok && u.Role == "admin"
}

// IsAllowed reports whether user may perform action on repo (e.g. "alice/app").
func (p *Policy) IsAllowed(username, repo string, action Action) bool {
	u, ok := p.Users[username]
	if !ok {
		return false
	}
	for _, r := range p.Policies {
		if r.Role != u.Role || !hasAction(r.Actions, action) {
			continue
		}
		for _, ns := range r.Namespaces {
			if matchNamespace(strings.ReplaceAll(ns, usernameVar, username), repo) {
				return true
			}
		}
	}
	return false
}

// FilterRepositories returns the repositories the user may pull.
func (p *Policy) FilterRepositories(username string, repos []string) []string {
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		if p.IsAllowed(username, repo, Pull) {
			out = append(out, repo)
		}
	}
	return out
}

func hasAction(actions []string, a Action) bool {
	for _, x := range actions {
		if Action(x) == a {
			return true
		}
	}
	return false
}

// matchNamespace supports "*" (everything), "prefix/*" (anything below prefix/)
// and exact repository names.
func matchNamespace(pattern, repo string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasSuffix(pattern, "/*"):
		return strings.HasPrefix(repo, strings.TrimSuffix(pattern, "*"))
	default:
		return pattern == repo
	}
}

// Store loads a policy file and transparently reloads it when its mtime or
// size changes. On a failed reload the last good policy stays active.
type Store struct {
	path string

	mu     sync.Mutex
	policy *Policy
	mtime  time.Time
	size   int64
}

var (
	sharedMu     sync.Mutex
	sharedStores = map[string]*Store{}
)

// SharedStore returns the process-wide Store for path, creating it on first
// use, so the auth backend and the catalog middleware see one policy.
func SharedStore(path string) (*Store, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if s, ok := sharedStores[path]; ok {
		return s, nil
	}
	s, err := NewStore(path)
	if err != nil {
		return nil, err
	}
	sharedStores[path] = s
	return s, nil
}

// NewStore creates a Store and performs the initial load.
func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	if _, err := s.Get(); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns the current policy, reloading the file if it changed.
func (s *Store) Get() (*Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fi, err := os.Stat(s.path)
	if err != nil {
		if s.policy != nil {
			return s.policy, nil
		}
		return nil, fmt.Errorf("rbac: stat policy: %w", err)
	}
	if s.policy != nil && fi.ModTime().Equal(s.mtime) && fi.Size() == s.size {
		return s.policy, nil
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		var p *Policy
		if p, err = ParsePolicy(data); err == nil {
			s.policy, s.mtime, s.size = p, fi.ModTime(), fi.Size()
			return p, nil
		}
	}
	if s.policy != nil {
		return s.policy, nil // keep last good policy
	}
	return nil, err
}
