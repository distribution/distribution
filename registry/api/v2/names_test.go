package v2

import (
	"strconv"
	"strings"
	"testing"
)

func TestRepositoryComponentNameRegexp(t *testing.T) {
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
		},
		{
			input: "simple/name",
		},
		{
			input: "library/ubuntu",
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
		},
		{
			input: "asdf$$^/aa",
			err:   ErrRepositoryNameComponentInvalid,
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
			input: "a-/a/a/a",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: strings.Repeat("a", 255),
		},
		{
			input: strings.Repeat("a", 256),
			err:   ErrRepositoryNameLong,
		},
		{
			input: "-foo/bar",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "foo/bar-",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "foo-/bar",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "foo/-bar",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "_foo/bar",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "foo/bar_",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "____/____",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "_docker/_docker",
			err:   ErrRepositoryNameComponentInvalid,
		},
		{
			input: "docker_/docker_",
			err:   ErrRepositoryNameComponentInvalid,
		},
	} {
		failf := func(format string, v ...interface{}) {
			t.Logf(strconv.Quote(testcase.input)+": "+format, v...)
			t.Fail()
		}

		if err := ValidateRepositoryName(testcase.input); err != testcase.err {
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

func TestRepositoryNameRegexp(t *testing.T) {
	for _, testcase := range []struct {
		input   string
		invalid bool
	}{
		{
			input: "short",
		},
		{
			input: "simple/name",
		},
		{
			input: "library/ubuntu",
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
			input:   "a/a/a/a/",
			invalid: true,
		},
		{
			input:   "a//a/a",
			invalid: true,
		},
		{
			input: "a",
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
			input:   "foo.com/",
			invalid: true,
		},
		{
			// currently not allowed by the regex
			input:   "foo.com:8080/bar",
			invalid: true,
		},
		{
			input: "foo.com/bar",
		},
		{
			input: "foo.com/bar/baz",
		},
		{
			input: "foo.com/bar/baz/quux",
		},
		{
			input: "blog.foo.com/bar/baz",
		},
		{
			input: "asdf",
		},
		{
			input:   "asdf$$^/aa",
			invalid: true,
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
			input:   "a-/a/a/a",
			invalid: true,
		},
		{
			input:   "-foo/bar",
			invalid: true,
		},
		{
			input:   "foo/bar-",
			invalid: true,
		},
		{
			input:   "foo-/bar",
			invalid: true,
		},
		{
			input:   "foo/-bar",
			invalid: true,
		},
		{
			input:   "_foo/bar",
			invalid: true,
		},
		{
			input:   "foo/bar_",
			invalid: true,
		},
		{
			input:   "____/____",
			invalid: true,
		},
		{
			input:   "_docker/_docker",
			invalid: true,
		},
		{
			input:   "docker_/docker_",
			invalid: true,
		},
	} {
		failf := func(format string, v ...interface{}) {
			t.Logf(strconv.Quote(testcase.input)+": "+format, v...)
			t.Fail()
		}

		matches := RepositoryNameRegexp.FindString(testcase.input) == testcase.input
		if matches == testcase.invalid {
			if testcase.invalid {
				failf("expected invalid repository name %s", testcase.input)
			} else {
				failf("expected valid repository name %s", testcase.input)
			}
		}
	}
}
