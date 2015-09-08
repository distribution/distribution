package reference

import (
	"strconv"
	"strings"
	"testing"

	"github.com/docker/distribution/digest"
)

func TestReferenceParse(t *testing.T) {
	// referenceTestcases is a unified set of testcases for
	// testing the parsing of references
	referenceTestcases := []struct {
		// input is the repository name or name component testcase
		input string
		// err is the error expected from Parse, or nil
		err error
		// repository is the string representation for the reference
		repository string
		// hostname is the hostname expected in the reference
		hostname string
		// tag is the tag for the reference
		tag string
		// digest is the digest for the reference (enforces digest reference)
		digest string
	}{
		{
			input:      "test_com",
			repository: "test_com",
		},
		{
			input:      "test.com:tag",
			repository: "test.com",
			tag:        "tag",
		},
		{
			input:      "test.com:5000",
			repository: "test.com",
			tag:        "5000",
		},
		{
			input:      "test.com/repo:tag",
			hostname:   "test.com",
			repository: "test.com/repo",
			tag:        "tag",
		},
		{
			input:      "test:5000/repo",
			hostname:   "test:5000",
			repository: "test:5000/repo",
		},
		{
			input:      "test:5000/repo:tag",
			hostname:   "test:5000",
			repository: "test:5000/repo",
			tag:        "tag",
		},
		{
			input:      "test:5000/repo@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			hostname:   "test:5000",
			repository: "test:5000/repo",
			digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:      "test:5000/repo:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			hostname:   "test:5000",
			repository: "test:5000/repo",
			tag:        "tag",
			digest:     "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:      "test:5000/repo",
			hostname:   "test:5000",
			repository: "test:5000/repo",
		},
		{
			input: "",
			err:   ErrNameEmpty,
		},
		{
			input: ":justtag",
			err:   ErrReferenceInvalidFormat,
		},
		{
			input: "@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			err:   ErrReferenceInvalidFormat,
		},
		{
			input: "validname@invaliddigest:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			err:   digest.ErrDigestUnsupported,
		},
		{
			input: strings.Repeat("a/", 128) + "a:tag",
			err:   ErrNameTooLong,
		},
		{
			input:      strings.Repeat("a/", 127) + "a:tag-puts-this-over-max",
			hostname:   "a",
			repository: strings.Repeat("a/", 127) + "a",
			tag:        "tag-puts-this-over-max",
		},
		{
			input: "aa/asdf$$^/aa",
			err:   ErrReferenceInvalidFormat,
		},
		{
			input:      "sub-dom1.foo.com/bar/baz/quux",
			hostname:   "sub-dom1.foo.com",
			repository: "sub-dom1.foo.com/bar/baz/quux",
		},
		{
			input:      "sub-dom1.foo.com/bar/baz/quux:some-long-tag",
			hostname:   "sub-dom1.foo.com",
			repository: "sub-dom1.foo.com/bar/baz/quux",
			tag:        "some-long-tag",
		},
		{
			input:      "b.gcr.io/test.example.com/my-app:test.example.com",
			hostname:   "b.gcr.io",
			repository: "b.gcr.io/test.example.com/my-app",
			tag:        "test.example.com",
		},
		{
			input:      "xn--n3h.com/myimage:xn--n3h.com", // ☃.com in punycode
			hostname:   "xn--n3h.com",
			repository: "xn--n3h.com/myimage",
			tag:        "xn--n3h.com",
		},
		{
			input:      "xn--7o8h.com/myimage:xn--7o8h.com@sha512:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // 🐳.com in punycode
			hostname:   "xn--7o8h.com",
			repository: "xn--7o8h.com/myimage",
			tag:        "xn--7o8h.com",
			digest:     "sha512:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:      "foo_bar.com:8080",
			repository: "foo_bar.com",
			tag:        "8080",
		},
		{
			input:      "foo/foo_bar.com:8080",
			hostname:   "foo",
			repository: "foo/foo_bar.com",
			tag:        "8080",
		},
	}
	for _, testcase := range referenceTestcases {
		failf := func(format string, v ...interface{}) {
			t.Logf(strconv.Quote(testcase.input)+": "+format, v...)
			t.Fail()
		}

		repo, err := Parse(testcase.input)
		if testcase.err != nil {
			if err == nil {
				failf("missing expected error: %v", testcase.err)
			} else if testcase.err != err {
				failf("mismatched error: got %v, expected %v", err, testcase.err)
			}
			continue
		} else if err != nil {
			failf("unexpected parse error: %v", err)
			continue
		}
		if repo.String() != testcase.input {
			failf("mismatched repo: got %q, expected %q", repo.String(), testcase.input)
		}

		if named, ok := repo.(Named); ok {
			if named.Name() != testcase.repository {
				failf("unexpected repository: got %q, expected %q", named.Name(), testcase.repository)
			}
			hostname, _ := SplitHostname(named)
			if hostname != testcase.hostname {
				failf("unexpected hostname: got %q, expected %q", hostname, testcase.hostname)
			}
		} else if testcase.repository != "" || testcase.hostname != "" {
			failf("expected named type, got %T", repo)
		}

		tagged, ok := repo.(Tagged)
		if testcase.tag != "" {
			if ok {
				if tagged.Tag() != testcase.tag {
					failf("unexpected tag: got %q, expected %q", tagged.Tag(), testcase.tag)
				}
			} else {
				failf("expected tagged type, got %T", repo)
			}
		} else if ok {
			failf("unexpected tagged type")
		}

		digested, ok := repo.(Digested)
		if testcase.digest != "" {
			if ok {
				if digested.Digest().String() != testcase.digest {
					failf("unexpected digest: got %q, expected %q", digested.Digest().String(), testcase.digest)
				}
			} else {
				failf("expected digested type, got %T", repo)
			}
		} else if ok {
			failf("unexpected digested type")
		}

	}
}

func TestSplitHostname(t *testing.T) {
	testcases := []struct {
		input    string
		hostname string
		name     string
	}{
		{
			input:    "test.com/foo",
			hostname: "test.com",
			name:     "foo",
		},
		{
			input:    "test_com/foo",
			hostname: "",
			name:     "test_com/foo",
		},
		{
			input:    "test:8080/foo",
			hostname: "test:8080",
			name:     "foo",
		},
		{
			input:    "test.com:8080/foo",
			hostname: "test.com:8080",
			name:     "foo",
		},
		{
			input:    "test-com:8080/foo",
			hostname: "test-com:8080",
			name:     "foo",
		},
		{
			input:    "xn--n3h.com:18080/foo",
			hostname: "xn--n3h.com:18080",
			name:     "foo",
		},
	}
	for _, testcase := range testcases {
		failf := func(format string, v ...interface{}) {
			t.Logf(strconv.Quote(testcase.input)+": "+format, v...)
			t.Fail()
		}

		named, err := ParseNamed(testcase.input)
		if err != nil {
			failf("error parsing name: %s", err)
		}
		hostname, name := SplitHostname(named)
		if hostname != testcase.hostname {
			failf("unexpected hostname: got %q, expected %q", hostname, testcase.hostname)
		}
		if name != testcase.name {
			failf("unexpected name: got %q, expected %q", name, testcase.name)
		}
	}
}
