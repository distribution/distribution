package storage

import (
	"fmt"
	"io"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
)

type setupEnv struct {
	ctx      context.Context
	driver   driver.StorageDriver
	expected []string
	registry distribution.Namespace
}

func setupFS(t *testing.T) *setupEnv {
	d := inmemory.New()
	c := []byte("")
	ctx := context.Background()
	registry, err := NewRegistry(ctx, d, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	rootpath, _ := pathFor(repositoriesRootPathSpec{})

	repos := []string{
		"/foo/a/_layers/1",
		"/foo/b/_layers/2",
		"/bar/c/_layers/3",
		"/bar/d/_layers/4",
		"/foo/d/in/_layers/5",
		"/an/invalid/repo",
		"/bar/d/_layers/ignored/dir/6",
	}

	for _, repo := range repos {
		if err := d.PutContent(ctx, rootpath+repo, c); err != nil {
			t.Fatalf("Unable to put to inmemory fs")
		}
	}

	expected := []string{
		"bar/c",
		"bar/d",
		"foo/a",
		"foo/b",
		"foo/d/in",
	}

	return &setupEnv{
		ctx:      ctx,
		driver:   d,
		expected: expected,
		registry: registry,
	}
}

func TestCatalog(t *testing.T) {
	env := setupFS(t)

	p := make([]string, 50)

	numFilled, err := env.registry.Repositories(env.ctx, p, "")

	if !testEq(p, env.expected, numFilled) {
		t.Errorf("Expected catalog repos err")
	}

	if err != io.EOF {
		t.Errorf("Catalog has more values which we aren't expecting")
	}
}

func TestCatalogInParts(t *testing.T) {
	env := setupFS(t)

	chunkLen := 2
	p := make([]string, chunkLen)

	numFilled, err := env.registry.Repositories(env.ctx, p, "")
	if err == io.EOF || numFilled != len(p) {
		t.Errorf("Expected more values in catalog")
	}

	if !testEq(p, env.expected[0:chunkLen], numFilled) {
		t.Errorf("Expected catalog first chunk err")
	}

	lastRepo := p[len(p)-1]
	numFilled, err = env.registry.Repositories(env.ctx, p, lastRepo)

	if err == io.EOF || numFilled != len(p) {
		t.Errorf("Expected more values in catalog")
	}

	if !testEq(p, env.expected[chunkLen:chunkLen*2], numFilled) {
		t.Errorf("Expected catalog second chunk err")
	}

	lastRepo = p[len(p)-1]
	numFilled, err = env.registry.Repositories(env.ctx, p, lastRepo)

	if err != io.EOF {
		t.Errorf("Catalog has more values which we aren't expecting")
	}

	if !testEq(p, env.expected[chunkLen*2:chunkLen*3-1], numFilled) {
		t.Errorf("Expected catalog third chunk err")
	}
}

func TestCatalogEnumerate(t *testing.T) {
	env := setupFS(t)

	var repos []string
	repositoryEnumerator := env.registry.(distribution.RepositoryEnumerator)
	err := repositoryEnumerator.Enumerate(env.ctx, func(repoName string) error {
		repos = append(repos, repoName)
		return nil
	})
	if err != nil {
		t.Errorf("Expected catalog enumerate err")
	}

	if len(repos) != len(env.expected) {
		t.Errorf("Expected catalog enumerate doesn't have correct number of values")
	}

	if !testEq(repos, env.expected, len(env.expected)) {
		t.Errorf("Expected catalog enumerate not over all values")
	}
}

func testEq(a, b []string, size int) bool {
	for cnt := 0; cnt < size-1; cnt++ {
		if a[cnt] != b[cnt] {
			return false
		}
	}
	return true
}

func setupBadWalkEnv(t *testing.T) *setupEnv {
	d := newBadListDriver()
	ctx := context.Background()
	registry, err := NewRegistry(ctx, d, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	return &setupEnv{
		ctx:      ctx,
		driver:   d,
		registry: registry,
	}
}

type badListDriver struct {
	driver.StorageDriver
}

var _ driver.StorageDriver = &badListDriver{}

func newBadListDriver() *badListDriver {
	return &badListDriver{StorageDriver: inmemory.New()}
}

func (d *badListDriver) List(ctx context.Context, path string) ([]string, error) {
	return nil, fmt.Errorf("List error")
}

func TestCatalogWalkError(t *testing.T) {
	env := setupBadWalkEnv(t)
	p := make([]string, 1)

	_, err := env.registry.Repositories(env.ctx, p, "")
	if err == io.EOF {
		t.Errorf("Expected catalog driver list error")
	}
}
