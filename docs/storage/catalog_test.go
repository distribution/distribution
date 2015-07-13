package storage

import (
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
	catalog  distribution.CatalogService
}

func setupFS(t *testing.T) *setupEnv {
	d := inmemory.New()
	c := []byte("")
	ctx := context.Background()
	registry := NewRegistryWithDriver(ctx, d, memory.NewInMemoryBlobDescriptorCacheProvider())
	rootpath, _ := defaultPathMapper.path(repositoriesRootPathSpec{})

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

	catalog := registry.Catalog(ctx)

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
		catalog:  catalog,
	}
}

func TestCatalog(t *testing.T) {
	env := setupFS(t)

	repos, more, _ := env.catalog.Get(100, "")

	if !testEq(repos, env.expected) {
		t.Errorf("Expected catalog repos err")
	}

	if more {
		t.Errorf("Catalog has more values which we aren't expecting")
	}
}

func TestCatalogInParts(t *testing.T) {
	env := setupFS(t)

	chunkLen := 2

	repos, more, _ := env.catalog.Get(chunkLen, "")
	if !testEq(repos, env.expected[0:chunkLen]) {
		t.Errorf("Expected catalog first chunk err")
	}

	if !more {
		t.Errorf("Expected more values in catalog")
	}

	lastRepo := repos[len(repos)-1]
	repos, more, _ = env.catalog.Get(chunkLen, lastRepo)

	if !testEq(repos, env.expected[chunkLen:chunkLen*2]) {
		t.Errorf("Expected catalog second chunk err")
	}

	if !more {
		t.Errorf("Expected more values in catalog")
	}

	lastRepo = repos[len(repos)-1]
	repos, more, _ = env.catalog.Get(chunkLen, lastRepo)

	if !testEq(repos, env.expected[chunkLen*2:chunkLen*3-1]) {
		t.Errorf("Expected catalog third chunk err")
	}

	if more {
		t.Errorf("Catalog has more values which we aren't expecting")
	}

}

func testEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for count := range a {
		if a[count] != b[count] {
			return false
		}
	}

	return true
}
