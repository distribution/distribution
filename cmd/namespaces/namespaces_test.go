package main

import (
	"bytes"
	"log"
	"sort"
	"strings"

	"testing"
)

func TestEntryLessEqual(t *testing.T) {
	for _, testcase := range []struct {
		// For each case, we expect a to be less than b. The inverse is
		// checked. If they are supposed to be equal, only the inverse
		// checked.
		a     Entry
		b     Entry
		equal bool
	}{
		{
			a:     mustEntry("docker.com/    push         https://registry.docker.com"),
			b:     mustEntry("docker.com/    push         https://registry.docker.com"),
			equal: true,
		},
		{
			a: mustEntry("docker.com/    push         https://aregistry.docker.com"),
			b: mustEntry("docker.com/    push         https://registry.docker.com"),
		},
		{
			a: mustEntry("docker.com/        pull         https://registry.docker.com"),
			b: mustEntry("docker.com/        push         https://registry.docker.com"),
		},
		{
			a: mustEntry("foo/               pull         http://foo.com"),
			b: mustEntry("*                  alias        docker.com/library/"),
		},
		{
			a: mustEntry("*/*                alias        docker.com/"),
			b: mustEntry("*                  alias        docker.com/library/"),
		},
		{
			a: mustEntry("foo/               pull         http://foo.com"),
			b: mustEntry("*/*                alias        docker.com/library/"),
		},
		{
			a: mustEntry("docker.com/        push         https://registry.docker.com"),
			b: mustEntry("foo/               pull         https://foo.com"),
		},
		{
			a: mustEntry("library/       alias    docker.com/library"),
			b: mustEntry("*/*            alias    docker.com/"),
		},
		{
			a: mustEntry("library/       alias    docker.com/library"),
			b: mustEntry("*              alias    docker.com/"),
		},
	} {
		if !testcase.equal && !EntryLess(testcase.a, testcase.b) {
			t.Fatalf("expected %v less than %v", testcase.a, testcase.b)
		}

		// Opposite must be true
		if EntryLess(testcase.b, testcase.a) {
			t.Fatalf("expected %v not less than %v", testcase.b, testcase.a)
		}

		if testcase.equal && !EntryEqual(testcase.a, testcase.b) {
			t.Fatalf("expected %v == %v", testcase.a, testcase.b)
		}

		if testcase.equal && !EntryEqual(testcase.b, testcase.a) {
			t.Fatalf("expected %v == %v", testcase.a, testcase.b)
		}
	}
}

func TestSortAndInsert(t *testing.T) {
	// completely unsorted junk
	namespaceConfig := `
docker.com/        push         https://registry.docker.com
*/*                alias        docker.com/
docker.com/        pull         https://registry.docker.com
docker.com/        pull         https://aregistry.docker.com
library/           alias        docker.com/library
*                  alias        docker.com/library/
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
a/b/c/             push         https://abc.com
a/b/               alias        a/b/c
*/b/               alias        a/b/c
*/a/               alias        a/b/c
a/*                alias        a/b/c
b/*                alias        a/b/c
abc/*              alias        a/b/c
`
	entries := mustEntries(namespaceConfig)

	var buf bytes.Buffer
	if err := WriteManager(&buf, &entries); err != nil {
		t.Fatalf("unexpected error writing entries: %v", err)
	}

	log.Println("sort ----- \n\n\n\n\n\n")
	sort.Sort(entries)
	log.Println("sort ----- \n\n\n\n\n\n")

	// Out expected output. Notice some important properties:
	// 1. aliases are partitioned to end.
	// 2.
	expected := strings.TrimSpace(`
a/b/c/         push     https://abc.com
docker.com/    pull     https://aregistry.docker.com
docker.com/    pull     https://registry.docker.com
docker.com/    push     https://registry.docker.com
foo/           pull     http://foo.com
a/b/           alias    a/b/c
library/       alias    docker.com/library
*/*            alias    docker.com/
*              alias    docker.com/library/
`)

	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Fatalf("\n%s\n != \n%s", strings.TrimSpace(buf.String()), strings.TrimSpace(expected))
	}
}

func TestInterestingCases(t *testing.T) {

	// Here we have an example conifguration with several interesting cases. Let's
	// build up some uses and query this configuration to make sure we have the
	// right idea.

	// 1. Resolving single component namespaces.
	// 2. Resolving dual component namespaces.
	// 3. Resolving different registry configurations for the same image name.
	//    (ie production vs staging). Similar to git remotes.

	// docker.com/                  namespace    asdf
	// docker.com/                  pull         https://mirror0.docker.com    mirror
	// docker.com/                  pull         https://mirror1.docker.com    mirror
	// docker.com/                  pull         https://mirror2.docker.com    mirror
	// docker.com/                  pull         https://registry.docker.com
	// docker.com/                  push         https://registry.docker.com
	// docker.com/                  push         https://registry.docker.com    v2
	// f/                           pull         http://foo.com
	// foo.com/                     pull         https://registry.foo.com/
	// foo.com/                     trust        http://foo.com/ca
	// foo.com/bar/                 push         http://localhost/
	// library/                     alias        docker.com/library
	// mycompany.com/               namespace    asdf
	// mycompany.com/               pull         https://registry.mycompany.com    v2
	// mycompany.com/               push         https://registry.mycompany.com    v2
	// mycompany.com/production/    push         http://registry-production.mycompany.com
	// production/                  push         http://registry.mycompany.com/production
	// redhat.com/                  pull         https://registry.docker.com    v2
	// redhat.com/                  push         https://registry.docker.com    v2
	// redhat/                      alias        redhat.com
	// staging/                     alias        mycompany.com
	// local/*                      alias        redhat.com/
	// */*                          alias        docker.com/
	// *                            alias        docker.com/library/
}

func mustEntry(s string) Entry {
	entry, err := ParseEntry(s)
	if err != nil {
		panic(err)
	}
	return entry
}

func mustEntries(s string) Entries {
	entries, err := ParseEntries(strings.NewReader(s))
	if err != nil {
		panic(err)
	}

	return entries
}
