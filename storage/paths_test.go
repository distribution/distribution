package storage

import "testing"

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
			spec: layerLinkPathSpec{
				name:   "foo/bar",
				tarSum: "tarsum.v1+test:abcdef",
			},
			expected: "/pathmapper-test/repositories/foo/bar/layers/tarsum/v1/test/abcdef",
		},
		{
			spec: layerIndexLinkPathSpec{
				tarSum: "tarsum.v1+test:abcdef",
			},
			expected: "/pathmapper-test/layerindex/tarsum/v1/test/abcdef",
		},
		{
			spec: blobPathSpec{
				alg:    "sha512",
				digest: "abcdefabcdefabcdef908909909",
			},
			expected: "/pathmapper-test/blob/sha512/ab/abcdefabcdefabcdef908909909",
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
