// Package rbac is a registry middleware that restricts the catalog
// (GET /v2/_catalog) to the repositories the authenticated user may pull.
//
// It must be used together with the "rbac" auth backend and the same policy file:
//
//	middleware:
//	  registry:
//	    - name: rbac
//	      options:
//	        policy: /auth/rbac_policy.yml
package rbac

import (
	"context"
	"fmt"
	"io"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/auth/rbac"
	middleware "github.com/distribution/distribution/v3/registry/middleware/registry"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/sirupsen/logrus"
)

// userNameKey is the context key under which the registry stores the
// authenticated user name (see registry/handlers/context.go).
const userNameKey = "auth.user.name"

// batchSize is how many unfiltered entries are read from storage per round
// while searching for entries the user may see.
const batchSize = 1000

func init() {
	if err := middleware.Register("rbac", newMiddleware); err != nil {
		logrus.Errorf("failed to register rbac registry middleware: %v", err)
	}
}

func newMiddleware(_ context.Context, ns distribution.Namespace, _ storagedriver.StorageDriver, options map[string]any) (distribution.Namespace, error) {
	path, ok := options["policy"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf(`"policy" must be set for rbac registry middleware`)
	}
	store, err := rbac.SharedStore(path)
	if err != nil {
		return nil, err
	}
	return &namespace{Namespace: ns, store: store}, nil
}

type namespace struct {
	distribution.Namespace
	store *rbac.Store
}

var (
	_ distribution.Namespace            = &namespace{}
	_ distribution.RepositoryEnumerator = &namespace{}
	_ distribution.RepositoryRemover    = &namespace{}
)

// Repositories fills repos with the user's visible repositories, in the same
// order and with the same paging contract as the wrapped namespace.
// It fails closed: no authenticated user means no repositories.
func (n *namespace) Repositories(ctx context.Context, repos []string, last string) (int, error) {
	if len(repos) == 0 {
		return n.Namespace.Repositories(ctx, repos, last)
	}
	user := dcontext.GetStringValue(ctx, userNameKey)
	if user == "" {
		return 0, io.EOF
	}
	policy, err := n.store.Get()
	if err != nil {
		return 0, err
	}

	filled := 0
	buf := make([]string, batchSize)
	for filled < len(repos) {
		got, err := n.Namespace.Repositories(ctx, buf, last)
		for _, repo := range buf[:got] {
			if policy.IsAllowed(user, repo, rbac.Pull) {
				repos[filled] = repo
				filled++
				if filled == len(repos) {
					// Output is full. Unread entries may remain in this batch even
					// if the wrapped namespace reported EOF, so signal "more" and
					// let the caller continue after repos[filled-1].
					return filled, nil
				}
			}
		}
		if err != nil {
			return filled, err // includes io.EOF: no more entries
		}
		if got == 0 {
			return filled, io.EOF
		}
		last = buf[got-1]
	}
	return filled, nil
}

// Enumerate passes through to the wrapped namespace (used by garbage collection).
func (n *namespace) Enumerate(ctx context.Context, ingester func(string) error) error {
	if e, ok := n.Namespace.(distribution.RepositoryEnumerator); ok {
		return e.Enumerate(ctx, ingester)
	}
	return fmt.Errorf("wrapped namespace does not support enumeration")
}

// Remove passes through to the wrapped namespace. Authorization for it is
// enforced by the auth backend (delete action on the repository).
func (n *namespace) Remove(ctx context.Context, name reference.Named) error {
	if r, ok := n.Namespace.(distribution.RepositoryRemover); ok {
		return r.Remove(ctx, name)
	}
	return fmt.Errorf("wrapped namespace does not support repository removal")
}

// Close lets App.Shutdown reach a wrapped pull-through cache.
func (n *namespace) Close() error {
	if c, ok := n.Namespace.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
