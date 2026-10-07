package inmemory

import (
	"testing"
	"time"
)

// TestFindSameNamedChild checks that find returns a directory itself, not its
// same-named child.
func TestFindSameNamedChild(t *testing.T) {
	root := &dir{common: common{p: "/", mod: time.Now()}}

	if _, err := root.mkdirs("/dirname/dirname"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.mkfile("/dirname/file"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		query string
		path  string
		isdir bool
	}{
		{"/dirname", "/dirname", true},
		{"dirname", "/dirname", true},
		{"/dirname/", "/dirname", true},
		{"/dirname/dirname", "/dirname/dirname", true},
		{"/dirname/file", "/dirname/file", false},

		// not found, so the closest existing parent is returned
		{"/dirname/dirname/dirname", "/dirname/dirname", true},
		{"/dirname/missing", "/dirname", true},
		{"/missing", "/", true},
	} {
		t.Run(tc.query, func(t *testing.T) {
			n := root.find(tc.query)
			if n.path() != tc.path {
				t.Errorf("find(%q).path() = %q, want %q", tc.query, n.path(), tc.path)
			}
			if n.isdir() != tc.isdir {
				t.Errorf("find(%q).isdir() = %v, want %v", tc.query, n.isdir(), tc.isdir)
			}
		})
	}
}
