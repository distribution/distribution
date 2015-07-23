package reference

import (
	"strconv"
	"strings"
	"testing"
)

func TestRepositoryNameRegexp(t *testing.T) {
	for _, testcase := range []struct {
		input string
		err   error
	}{
		{
			input: "",
			err:   ErrRepositoryNameEmpty,
		},
		{
			input: "short",
			err:   ErrRepositoryNameMissingHostname,
		},
		{
			input: "simple/name",
		},
		{
			input: "docker/stevvooe/app",
		},
		{
			input: "aa/aa/aa/aa/aa/aa/aa/aa/aa/bb/bb/bb/bb/bb/bb",
		},
		{
			input: "aa/aa/bb/bb/bb",
		},
		{
			input: "a/a/a/b/b",
		},
		{
			input: "a/a/a/a/",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "a//a/a",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "a",
			err:   ErrRepositoryNameMissingHostname,
		},
		{
			input: "a/aa",
		},
		{
			input: "aa/a",
		},
		{
			input: "a/aa/a",
		},
		{
			input: "foo.com/bar/baz",
		},
		{
			input: "blog.foo.com/bar/baz",
		},
		{
			input: "asdf",
			err:   ErrRepositoryNameMissingHostname,
		},
		{
			input: "aa/asdf$$^/aa",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "asdf$$^/aa",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "aa-a/aa",
		},
		{
			input: "aa/aa",
		},
		{
			input: "a-a/a-a",
		},
		{
			input: "a",
			err:   ErrRepositoryNameMissingHostname,
		},
		{
			input: "a/image",
		},
		{
			input: "a-/a/a/a",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "a/a-/a/a/a",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			// total length = 255
			input: "a/" + strings.Repeat("a", 253),
		},
		{
			// total length = 256
			input: "b/" + strings.Repeat("a", 254),
			err:   ErrRepositoryNameLong,
		},
		{
			input: "-foo/bar",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "foo/bar-",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "foo-/bar",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "foo/-bar",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "_foo/bar",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "foo/bar_",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "____/____",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "_docker/_docker",
			err:   ErrRepositoryNameHostnameInvalid,
		},
		{
			input: "docker_/docker_",
			err:   ErrRepositoryNameHostnameInvalid,
		},
	} {
		failf := func(format string, v ...interface{}) {
			t.Logf(strconv.Quote(testcase.input)+": "+format, v...)
			t.Fail()
		}

		if _, err := NewRepository(testcase.input); err != testcase.err {
			if testcase.err != nil {
				if err != nil {
					failf("unexpected error for invalid repository: got %v, expected %v", err, testcase.err)
				} else {
					failf("expected invalid repository: %v", testcase.err)
				}
			} else {
				if err != nil {
					// Wrong error returned.
					failf("unexpected error validating repository name: %v, expected %v", err, testcase.err)
				} else {
					failf("unexpected error validating repository name: %v", err)
				}
			}
		}
	}
}
