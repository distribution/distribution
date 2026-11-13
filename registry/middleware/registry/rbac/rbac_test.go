package rbac

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/reference"
	"golang.org/x/crypto/bcrypt"
)

// fakeNS implements the paging contract of storage.registry.Repositories.
type fakeNS struct {
	distribution.Namespace
	repos   []string
	removed []string
}

func (f *fakeNS) Repositories(_ context.Context, out []string, last string) (int, error) {
	if len(out) == 0 {
		return 0, fmt.Errorf("attempted to list 0 repositories")
	}
	n := 0
	for _, r := range f.repos {
		if r <= last {
			continue
		}
		out[n] = r
		n++
		if n == len(out) {
			return n, nil
		}
	}
	return n, io.EOF
}

func (f *fakeNS) Remove(_ context.Context, name reference.Named) error {
	f.removed = append(f.removed, name.Name())
	return nil
}

type userCtx struct {
	context.Context
	user string
}

func (u userCtx) Value(k any) any {
	if k == userNameKey {
		return u.user
	}
	return u.Context.Value(k)
}

func setup(t *testing.T, repos []string) (*namespace, *fakeNS) {
	t.Helper()
	h := func(pw string) string {
		b, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
		return string(b)
	}
	y := "users:\n" +
		"  alice: {password: '" + h("x") + "', role: user}\n" +
		"  bob: {password: '" + h("x") + "', role: user}\n" +
		"  root: {password: '" + h("x") + "', role: admin}\n" +
		"policies:\n" +
		"  - {role: admin, namespaces: ['*'], actions: [push, pull, delete]}\n" +
		"  - {role: user, namespaces: ['${username}/*'], actions: [push, pull, delete]}\n" +
		"  - {role: user, namespaces: ['public/*'], actions: [pull]}\n"
	path := filepath.Join(t.TempDir(), "p.yml")
	if err := os.WriteFile(path, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	inner := &fakeNS{repos: repos}
	ns, err := newMiddleware(context.Background(), inner, nil, map[string]any{"policy": path})
	if err != nil {
		t.Fatal(err)
	}
	return ns.(*namespace), inner
}

// listAll pages through the catalog exactly like registry/handlers/catalog.go.
func listAll(t *testing.T, ns *namespace, user string, pageSize int) []string {
	t.Helper()
	ctx := userCtx{context.Background(), user}
	var all []string
	last := ""
	for i := 0; i < 5000; i++ {
		buf := make([]string, pageSize)
		n, err := ns.Repositories(ctx, buf, last)
		all = append(all, buf[:n]...)
		if err == io.EOF {
			return all
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no progress but no EOF")
		}
		last = buf[n-1]
	}
	t.Fatal("paging did not terminate")
	return nil
}

func TestCatalogFilteredPerUser(t *testing.T) {
	ns, _ := setup(t, []string{"alice/a", "alice/b", "bob/a", "other/x", "public/base", "zed/z"})
	for _, size := range []int{1, 2, 3, 100} {
		if got, want := listAll(t, ns, "alice", size), []string{"alice/a", "alice/b", "public/base"}; !reflect.DeepEqual(got, want) {
			t.Errorf("page=%d alice=%v want %v", size, got, want)
		}
		if got, want := listAll(t, ns, "bob", size), []string{"bob/a", "public/base"}; !reflect.DeepEqual(got, want) {
			t.Errorf("page=%d bob=%v want %v", size, got, want)
		}
		if got := listAll(t, ns, "root", size); len(got) != 6 {
			t.Errorf("page=%d admin=%v", size, got)
		}
		if got := listAll(t, ns, "ghost", size); len(got) != 0 {
			t.Errorf("page=%d unknown user=%v", size, got)
		}
	}
}

func TestCatalogFailsClosedWithoutUser(t *testing.T) {
	ns, _ := setup(t, []string{"alice/a", "public/p"})
	n, err := ns.Repositories(context.Background(), make([]string, 10), "")
	if n != 0 || err != io.EOF {
		t.Errorf("n=%d err=%v; want 0, io.EOF", n, err)
	}
}

// Many hidden entries before the first visible one must not produce an empty
// page with no EOF (which would make clients stop early or loop).
func TestCatalogSkipsLongRunsOfHiddenRepos(t *testing.T) {
	var repos []string
	for i := 0; i < 2500; i++ {
		repos = append(repos, fmt.Sprintf("bob/r%05d", i))
	}
	repos = append(repos, "public/p")
	sort.Strings(repos)
	ns, _ := setup(t, repos)
	if got, want := listAll(t, ns, "alice", 1), []string{"public/p"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCatalogZeroLengthDelegates(t *testing.T) {
	ns, _ := setup(t, nil)
	if _, err := ns.Repositories(userCtx{context.Background(), "alice"}, nil, ""); err == nil {
		t.Error("expected the wrapped namespace's error for 0-length request")
	}
}

func TestPassThroughs(t *testing.T) {
	ns, inner := setup(t, nil)
	name, _ := reference.WithName("alice/app")
	if err := ns.Remove(context.Background(), name); err != nil || len(inner.removed) != 1 {
		t.Errorf("Remove: err=%v removed=%v", err, inner.removed)
	}
	if err := ns.Enumerate(context.Background(), func(string) error { return nil }); err == nil {
		t.Error("Enumerate should report unsupported for a wrapped namespace without it")
	}
	if err := ns.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMiddlewareOptions(t *testing.T) {
	for _, opts := range []map[string]any{{}, {"policy": ""}, {"policy": "/does/not/exist"}} {
		if _, err := newMiddleware(context.Background(), &fakeNS{}, nil, opts); err == nil {
			t.Errorf("expected error for %v", opts)
		}
	}
}
