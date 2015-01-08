package storage

import (
	"testing"

	"github.com/docker/distribution/digest"
)

func TestPathMapper(t *testing.T) {
	pm := &pathMapper{
		root: "/pathmapper-test",
	}

	for _, testcase := range []struct {
		spec     pathSpec
		expected string
		err      error
	}{
		{
			spec: manifestPathSpec{
				name: "foo/bar",
				tag:  "thetag",
			},
			expected: "/pathmapper-test/repositories/foo/bar/manifests/thetag",
		},
		{
			spec: layerLinkPathSpec{
				name:   "foo/bar",
				digest: digest.Digest("tarsum.v1+test:abcdef"),
			},
			expected: "/pathmapper-test/repositories/foo/bar/layers/tarsum/v1/test/ab/abcdef",
		},
		{
			spec: blobPathSpec{
				digest: digest.Digest("tarsum.dev+sha512:abcdefabcdefabcdef908909909"),
			},
			expected: "/pathmapper-test/blob/tarsum/dev/sha512/ab/abcdefabcdefabcdef908909909",
		},
		{
			spec: blobPathSpec{
				digest: digest.Digest("tarsum.v1+sha256:abcdefabcdefabcdef908909909"),
			},
			expected: "/pathmapper-test/blob/tarsum/v1/sha256/ab/abcdefabcdefabcdef908909909",
		},
		{
			spec: uploadDataPathSpec{
				name: "foo/bar",
				uuid: "asdf-asdf-asdf-adsf",
			},
			expected: "/pathmapper-test/repositories/foo/bar/uploads/asdf-asdf-asdf-adsf/data",
		},
		{
			spec: uploadStartedAtPathSpec{
				name: "foo/bar",
				uuid: "asdf-asdf-asdf-adsf",
			},
			expected: "/pathmapper-test/repositories/foo/bar/uploads/asdf-asdf-asdf-adsf/startedat",
		},
	} {
		p, err := pm.path(testcase.spec)
		if err != nil {
			t.Fatal(err)
		}

		if p != testcase.expected {
			t.Fatalf("unexpected path generated: %q != %q", p, testcase.expected)
		}
	}
}
