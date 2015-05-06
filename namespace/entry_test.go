package namespace

import (
	"bytes"
	"log"
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
			a: mustEntry("docker.com/        push         https://registry.docker.com"),
			b: mustEntry("foo/               pull         https://foo.com"),
		},
	} {
		if !testcase.equal && !entryLess(testcase.a, testcase.b) {
			t.Fatalf("expected %v less than %v", testcase.a, testcase.b)
		}

		// Opposite must be true
		if entryLess(testcase.b, testcase.a) {
			t.Fatalf("expected %v not less than %v", testcase.b, testcase.a)
		}

		if testcase.equal && !entryEqual(testcase.a, testcase.b) {
			t.Fatalf("expected %v == %v", testcase.a, testcase.b)
		}

		if testcase.equal && !entryEqual(testcase.b, testcase.a) {
			t.Fatalf("expected %v == %v", testcase.a, testcase.b)
		}
	}
}

func TestEntryInsert(t *testing.T) {
	// completely unsorted junk
	namespaceConfig := `
docker.com/        push         https://registry.docker.com
docker.com/        pull         https://registry.docker.com
docker.com/        pull         https://aregistry.docker.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
foo/               pull         http://foo.com
a/b/c/             push         https://abc.com
`
	entries := mustEntries(namespaceConfig)

	var buf bytes.Buffer
	if err := WriteEntries(&buf, entries); err != nil {
		t.Fatalf("unexpected error writing entries: %v", err)
	}

	// Out expected output.
	expected := strings.TrimSpace(`
a/b/c/         push    https://abc.com
docker.com/    pull    https://aregistry.docker.com
docker.com/    pull    https://registry.docker.com
docker.com/    push    https://registry.docker.com
foo/           pull    http://foo.com
`)

	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Fatalf("\n%q\n != \n%q", strings.TrimSpace(buf.String()), strings.TrimSpace(expected))
	}
}

func TestEntryNew(t *testing.T) {
	good, err := NewEntry("valid/scope", "pull", "endpoint", "flag")
	if err != nil {
		t.Fatalf("Error parsing entry: %s", err)
	}
	if string(good.Scope().(scope)) != "valid/scope" {
		t.Fatalf("Wrong scope: %q, expecting %q", string(good.Scope().(scope)), "valid/scope")
	}
	if good.Action() != "pull" {
		t.Fatalf("Wrong action: %q, expecting %q", good.Action(), "pull")
	}
	args := good.Args()
	if len(args) != 2 {
		t.Fatalf("Wrong number of args: %d, expecting 2", len(args))
	}
	if args[0] != "endpoint" {
		t.Fatalf("Wrong argument: %q, expecting %q", args[0], "endpoint")
	}
	if args[1] != "flag" {
		t.Fatalf("Wrong argument: %q, expecting %q", args[1], "flag")
	}
	if _, err := NewEntry("", "pull", "endpoint", "flag"); err == nil {
		t.Fatal("Expected error creating entry without scope")
	}
	if _, err := NewEntry("valid/scope", "badaction", "endpoint", "flag"); err == nil {
		t.Fatal("Expected error creating entry with bad action")
	}
	good, err = NewEntry("valid/scope", "namespace")
	if err != nil {
		t.Fatalf("Error parsing entry: %s", err)
	}
	if len(good.Args()) != 0 {
		t.Fatalf("Wrong number of args: %d, expecting 0", len(good.Args()))
	}
}

func TestEntryFind(t *testing.T) {
	entries := mustEntries(`
a/b/c/         push    https://abc.com
docker.com/    pull    https://aregistry.docker.com
docker.com/    pull    https://registry.docker.com
docker.com/    push    https://registry.docker.com
docker.com/sub push    https://registry.docker.com
foo/           pull    http://foo.com
`)
	found, err := entries.Find("docker.com")
	if err != nil {
		log.Fatalf("Error finding entries: %s", err)
	}

	var buf bytes.Buffer
	if err := WriteEntries(&buf, found); err != nil {
		t.Fatalf("unexpected error writing entries: %v", err)
	}

	// Out expected output.
	expected := strings.TrimSpace(`
docker.com/    pull    https://aregistry.docker.com
docker.com/    pull    https://registry.docker.com
docker.com/    push    https://registry.docker.com
`)
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Fatalf("\n%q\n != \n%q", strings.TrimSpace(buf.String()), strings.TrimSpace(expected))
	}
}

func TestEntryRemove(t *testing.T) {
	entries := mustEntries(`
a/b/c/         push    https://abc.com
docker.com/    pull    https://aregistry.docker.com
docker.com/    pull    https://registry.docker.com
docker.com/    push    https://registry.docker.com
docker.com/sub push    https://registry.docker.com
foo/           pull    http://foo.com
`)
	rm1 := mustEntry("docker.com/ pull https://aregistry.docker.com")
	rm2 := mustEntry("foo/ pull http://foo.com")
	rm3 := mustEntry("docker.com/sub push https://registry.docker.com")
	rm4 := mustEntry("docker.com/bad push https://registry.docker.com")

	if err := entries.Remove(rm1); err != nil {
		t.Fatalf("Error removing entry: %s", err)
	}
	if err := entries.Remove(rm2); err != nil {
		t.Fatalf("Error removing entry: %s", err)
	}
	if err := entries.Remove(rm3); err != nil {
		t.Fatalf("Error removing entry: %s", err)
	}
	if err := entries.Remove(rm4); err != nil {
		t.Fatalf("Error removing non-existant entry: %s", err)
	}

	found, err := entries.Find("docker.com")
	if err != nil {
		log.Fatalf("Error finding entries: %s", err)
	}

	var buf bytes.Buffer
	if err := WriteEntries(&buf, found); err != nil {
		t.Fatalf("unexpected error writing entries: %v", err)
	}

	// Out expected output.
	expected := strings.TrimSpace(`
docker.com/    pull    https://registry.docker.com
docker.com/    push    https://registry.docker.com
`)
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(expected) {
		t.Fatalf("\n%q\n != \n%q", strings.TrimSpace(buf.String()), strings.TrimSpace(expected))
	}
}

func mustEntry(s string) Entry {
	entry, err := parseEntry(s)
	if err != nil {
		panic(err)
	}
	return entry
}

func mustEntries(s string) *Entries {
	entries, err := ParseEntries(strings.NewReader(s))
	if err != nil {
		panic(err)
	}

	return entries
}
