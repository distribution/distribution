package namespace

import (
	"fmt"
	"strings"
	"testing"
)

func entryString(entry Entry) string {
	return fmt.Sprintf("%s %s %s", entry.scope, entry.action, strings.Join(entry.args, " "))
}

func assertEntryEqual(t *testing.T, actual, expected Entry) {
	s1 := entryString(actual)
	s2 := entryString(expected)
	if s1 != s2 {
		t.Fatalf("Unexpected entry\n\tExpected: %s\n\tActual:   %s", s2, s1)
	}
}

func assertResolution(t *testing.T, r Resolver, name, matchString string) {
	matchEntries := mustEntries(matchString)

	entries, err := r.Resolve(name)
	if err != nil {
		t.Fatalf("Error resolving namespace: %s", err)
	}

	if len(entries.entries) != len(matchEntries.entries) {
		t.Fatalf("Unexpected number of entries for %q: %d, expected %d", name, len(entries.entries), len(matchEntries.entries))
	}

	for i := range entries.entries {
		assertEntryEqual(t, entries.entries[i], matchEntries.entries[i])
	}
}

// Test case
// No base + discovery
// Base with extension discovery
// Base with no scoped discovery
func TestMultiResolution(t *testing.T) {
	entries1 := mustEntries(`
docker.com/        push         https://registry.base.docker.com
docker.com/        pull         https://registry.base.docker.com
docker.com/        index        https://search.base.docker.com
docker.com/other   index           https://search.docker.com/v1/
docker.com/other   namespace       docker.com/
docker.com/other/block   namespace
docker.com/other/sub   index    https://search.sub.docker.com
docker.com/other/sub   namespace   docker.com/other
docker.com/extend  namespace       docker.com/
docker.com/img/sub  pull       https://mirror.base.docker.com
docker.com/img/sub  namespace       docker.com/img
`)
	entries2 := mustEntries(`
docker.com/img        push         https://registry.docker.com
docker.com/img        pull         https://registry.docker.com
docker.com/img        index        https://search.docker.com
`)

	resolver := NewMultiResolver(NewSimpleResolver(entries1, false), NewSimpleResolver(entries2, false))

	assertResolution(t, resolver, "docker.com/img", `
docker.com/img        push         https://registry.docker.com
docker.com/img        pull         https://registry.docker.com
docker.com/img        index        https://search.docker.com
`)

	assertResolution(t, resolver, "docker.com/other", `
docker.com/other   index           https://search.docker.com/v1/
docker.com/other   namespace       docker.com/
docker.com/        push         https://registry.base.docker.com
docker.com/        pull         https://registry.base.docker.com
docker.com/        index        https://search.base.docker.com
`)

	assertResolution(t, resolver, "docker.com/other/sub", `
docker.com/other/sub   index    https://search.sub.docker.com
docker.com/other/sub   namespace   docker.com/other
docker.com/other   index           https://search.docker.com/v1/
docker.com/other   namespace       docker.com/
docker.com/        push         https://registry.base.docker.com
docker.com/        pull         https://registry.base.docker.com
docker.com/        index        https://search.base.docker.com
`)

	assertResolution(t, resolver, "docker.com/img/sub", `
docker.com/img/sub  pull       https://mirror.base.docker.com
docker.com/img/sub  namespace       docker.com/img
docker.com/img        push         https://registry.docker.com
docker.com/img        pull         https://registry.docker.com
docker.com/img        index        https://search.docker.com
`)

	assertResolution(t, resolver, "docker.com/other/block", `
docker.com/other/block   namespace
`)
}

func TestExtendResolution(t *testing.T) {
	entries1 := mustEntries(`
docker.com/extend        push         https://registry.base.docker.com
docker.com/extend        pull         https://registry.base.docker.com
docker.com/extend        index        https://search.base.docker.com
docker.com/extend/other   namespace       docker.com/extend
`)
	entries2 := mustEntries(`
docker.com/img        push         https://registry.docker.com
docker.com/img        pull         https://registry.docker.com
docker.com/img        index        https://search.docker.com
docker.com/extend        push         https://mirror.docker.com
docker.com/extend        pull         https://mirror.docker.com
docker.com/extend        index        https://search.mirror.docker.com
`)

	resolver := NewExtendResolver(entries2, NewSimpleResolver(entries1, false))
	assertResolution(t, resolver, "docker.com/img", "")
	assertResolution(t, resolver, "docker.com/extend", `
docker.com/extend        push         https://registry.base.docker.com
docker.com/extend        pull         https://registry.base.docker.com
docker.com/extend        index        https://search.base.docker.com
docker.com/extend        push         https://mirror.docker.com
docker.com/extend        pull         https://mirror.docker.com
docker.com/extend        index        https://search.mirror.docker.com
`)
	assertResolution(t, resolver, "docker.com/extend/other", `
docker.com/extend        push         https://registry.base.docker.com
docker.com/extend        pull         https://registry.base.docker.com
docker.com/extend        index        https://search.base.docker.com
docker.com/extend/other   namespace       docker.com/extend
`)
}

func TestSimpleResolver(t *testing.T) {
	entries := mustEntries(`
docker.com/extend        index        https://search.base.docker.com
docker.com/sibling        index        https://search.base.docker.com
docker.com/extend/other   namespace       docker.com/extend
`)
	resolver := NewSimpleResolver(entries, false)
	assertResolution(t, resolver, "docker.com/extend/other", `
docker.com/extend        index        https://search.base.docker.com
docker.com/extend/other   namespace       docker.com/extend
`)
}

func TestSimpleResolverSiblings(t *testing.T) {
	entries := mustEntries(`
docker.com/extend        index        https://search.base.docker.com
docker.com/sibling        index        https://search.base.docker.com
docker.com/extend/other   namespace       docker.com/sibling
`)
	resolver := NewSimpleResolver(entries, false)
	if _, err := resolver.Resolve("docker.com/extend/other"); err == nil {
		t.Fatal("Expected error resolving with sibling namespace entry")
	}
}

func TestSimpleResolverPrefix(t *testing.T) {
	entries := mustEntries(`
docker.com/extend        index        https://search.base.docker.com
docker.com/extend/other   namespace
docker.com/sibling        index        https://search.base.docker.com
`)

	resolver := NewSimpleResolver(entries, false)
	assertResolution(t, resolver, "docker.com/extend/other", `
docker.com/extend/other   namespace
`)
	assertResolution(t, resolver, "docker.com/extend/other/sub", "")

	resolver = NewSimpleResolver(entries, true)
	assertResolution(t, resolver, "docker.com/extend/other", `
docker.com/extend/other   namespace
`)
	assertResolution(t, resolver, "docker.com/extend/other/sub", `
docker.com/extend/other   namespace
`)
	assertResolution(t, resolver, "docker.com/extend/sub", `
docker.com/extend        index        https://search.base.docker.com
`)
}
