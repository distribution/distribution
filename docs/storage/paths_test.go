package storage

import (
	"testing"

	"github.com/docker/distribution/digest"
)

func TestPathMapper(t *testing.T) {
	for _, testcase := range []struct {
		spec     pathSpec
		expected string
		err      error
	}{
		{
			spec: manifestRevisionPathSpec{
				name:     "foo/bar",
				revision: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789",
		},
		{
			spec: manifestRevisionLinkPathSpec{
				name:     "foo/bar",
				revision: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789/link",
		},
		{
			spec: manifestSignatureLinkPathSpec{
				name:      "foo/bar",
				revision:  "sha256:abcdef0123456789",
				signature: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789/signatures/sha256/abcdef0123456789/link",
		},
		{
			spec: manifestSignaturesPathSpec{
				name:     "foo/bar",
				revision: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789/signatures",
		},
		{
			spec: manifestTagsPathSpec{
				name: "foo/bar",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags",
		},
		{
			spec: manifestTagPathSpec{
				name: "foo/bar",
				tag:  "thetag",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag",
		},
		{
			spec: manifestTagCurrentPathSpec{
				name: "foo/bar",
				tag:  "thetag",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/current/link",
		},
		{
			spec: manifestTagIndexPathSpec{
				name: "foo/bar",
				tag:  "thetag",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/index",
		},
		{
			spec: manifestTagIndexEntryPathSpec{
				name:     "foo/bar",
				tag:      "thetag",
				revision: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/index/sha256/abcdef0123456789",
		},
		{
			spec: manifestTagIndexEntryLinkPathSpec{
				name:     "foo/bar",
				tag:      "thetag",
				revision: "sha256:abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/index/sha256/abcdef0123456789/link",
		},
		{
			spec: layerLinkPathSpec{
				name:   "foo/bar",
				digest: "tarsum.v1+test:abcdef",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_layers/tarsum/v1/test/abcdef/link",
		},
		{
			spec: blobDataPathSpec{
				digest: digest.Digest("tarsum.dev+sha512:abcdefabcdefabcdef908909909"),
			},
			expected: "/docker/registry/v2/blobs/tarsum/dev/sha512/ab/abcdefabcdefabcdef908909909/data",
		},
		{
			spec: blobDataPathSpec{
				digest: digest.Digest("tarsum.v1+sha256:abcdefabcdefabcdef908909909"),
			},
			expected: "/docker/registry/v2/blobs/tarsum/v1/sha256/ab/abcdefabcdefabcdef908909909/data",
		},

		{
			spec: uploadDataPathSpec{
				name: "foo/bar",
				id:   "asdf-asdf-asdf-adsf",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_uploads/asdf-asdf-asdf-adsf/data",
		},
		{
			spec: uploadStartedAtPathSpec{
				name: "foo/bar",
				id:   "asdf-asdf-asdf-adsf",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_uploads/asdf-asdf-asdf-adsf/startedat",
		},
	} {
		p, err := pathFor(testcase.spec)
		if err != nil {
			t.Fatalf("unexpected generating path (%T): %v", testcase.spec, err)
		}

		if p != testcase.expected {
			t.Fatalf("unexpected path generated (%T): %q != %q", testcase.spec, p, testcase.expected)
		}
	}

	// Add a few test cases to ensure we cover some errors

	// Specify a path that requires a revision and get a digest validation error.
	badpath, err := pathFor(manifestSignaturesPathSpec{
		name: "foo/bar",
	})

	if err == nil {
		t.Fatalf("expected an error when mapping an invalid revision: %s", badpath)
	}

}
