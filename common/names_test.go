package common

import (
	"testing"
)

func TestRepositoryNameRegexp(t *testing.T) {
	for _, testcase := range []struct {
		input string
		valid bool
	}{
		{
			input: "simple/name",
			valid: true,
		},
		{
			input: "library/ubuntu",
			valid: true,
		},
		{
			input: "docker/stevvooe/app",
			valid: true,
		},
		{
			input: "aa/aa/aa/aa/aa/aa/aa/aa/aa/bb/bb/bb/bb/bb/bb",
			valid: true,
		},
		{
			input: "a/a/a/a/a/a/b/b/b/b",
			valid: false,
		},
		{
			input: "a/a/a/a/",
			valid: false,
		},
		{
			input: "foo.com/bar/baz",
			valid: true,
		},
		{
			input: "blog.foo.com/bar/baz",
			valid: true,
		},
		{
			input: "asdf",
			valid: false,
		},
		{
			input: "asdf$$^/",
			valid: false,
		},
	} {
		if RepositoryNameRegexp.MatchString(testcase.input) != testcase.valid {
			status := "invalid"
			if testcase.valid {
				status = "valid"
			}

			t.Fatalf("expected %q to be %s repository name", testcase.input, status)
		}
	}
}
