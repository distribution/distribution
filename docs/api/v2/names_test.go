package v2

import (
	"strconv"
	"strings"
	"testing"
)

var (
	// regexpTestcases is a unified set of testcases for
	// TestValidateRepositoryName and TestRepositoryNameRegexp.
	// Some of them are valid inputs for one and not the other.
	regexpTestcases = []struct {
		// input is the repository name or name component testcase
		input string
		// err is the error expected from ValidateRepositoryName, or nil
		err error
		// invalid should be true if the testcase is *not* expected to
		// match RepositoryNameRegexp
		invalid bool
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
			input:   "a/a/a/a/",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "a//a/a",
			err:     ErrRepositoryNameComponentInvalid,
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
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			// TODO: this testcase should be valid once we switch to
			// the reference package.
			input:   "foo.com:8080/bar",
			err:     ErrRepositoryNameComponentInvalid,
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
			err:     ErrRepositoryNameComponentInvalid,
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
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input: strings.Repeat("a", 255),
		},
		{
			input: strings.Repeat("a", 256),
			err:   ErrRepositoryNameLong,
		},
		{
			input:   "-foo/bar",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "foo/bar-",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "foo-/bar",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "foo/-bar",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "_foo/bar",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "foo/bar_",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "____/____",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "_docker/_docker",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "docker_/docker_",
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input: "b.gcr.io/test.example.com/my-app", // embedded domain component
		},
		// TODO(stevvooe): The following is a punycode domain name that we may
		// want to allow in the future. Currently, this is not allowed but we
		// may want to change this in the future. Adding this here as invalid
		// for the time being.
		{
			input:   "xn--n3h.com/myimage", // http://☃.com in punycode
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
		{
			input:   "xn--7o8h.com/myimage", // http://🐳.com in punycode
			err:     ErrRepositoryNameComponentInvalid,
			invalid: true,
		},
	}
)

// TestValidateRepositoryName tests the ValidateRepositoryName function,
// which uses RepositoryNameComponentAnchoredRegexp for validation
func TestValidateRepositoryName(t *testing.T) {
	for _, testcase := range regexpTestcases {
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
	for _, testcase := range regexpTestcases {
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
