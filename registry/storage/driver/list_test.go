package driver

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

type sharedListDriver struct {
	StorageDriver
	entries []string
	err     error
}

func (d *sharedListDriver) List(context.Context, string) ([]string, error) {
	return d.entries, d.err
}

func TestListNamesFallback(t *testing.T) {
	ctx := context.Background()
	d := &sharedListDriver{entries: []string{"/tags/z", "/tags/a", "/tags/directory/"}}
	want := append([]string(nil), d.entries...)
	names, err := ListNames(ctx, d, "/tags")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"z", "a", ""}) {
		t.Fatalf("unexpected names: %q", names)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(d.entries, want) {
		t.Fatalf("ListNames modified the driver's slice: %q", d.entries)
	}
	next, err := ListNames(ctx, d, "/tags")
	if err != nil {
		t.Fatal(err)
	}
	next[0] = "changed"
	if !reflect.DeepEqual(names, []string{"", "a", "z"}) {
		t.Fatalf("a later result changed an earlier result: %q", names)
	}

	d.err = errors.New("listing failed")
	if got, err := ListNames(ctx, d, "/tags"); got != nil || err != d.err {
		t.Fatalf("got %q, %v; want nil, %v", got, err, d.err)
	}
	d.err, d.entries = nil, nil
	if got, err := ListNames(ctx, d, "/tags"); got == nil || len(got) != 0 || err != nil {
		t.Fatalf("empty listing: got %q, %v", got, err)
	}
}
