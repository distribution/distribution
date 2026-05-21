package storage

import (
	"testing"

	"github.com/opencontainers/go-digest"
)

func TestPathMapper(t *testing.T) {
	for _, testcase := range []struct {
		spec     pathSpec
		expected string
	}{
		{
			spec: manifestRevisionPathSpec{
				name:     "foo/bar",
				revision: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			spec: manifestRevisionLinkPathSpec{
				name:     "foo/bar",
				revision: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789/link",
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
				revision: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/index/sha256/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			spec: manifestTagIndexEntryLinkPathSpec{
				name:     "foo/bar",
				tag:      "thetag",
				revision: "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
			expected: "/docker/registry/v2/repositories/foo/bar/_manifests/tags/thetag/index/sha256/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789/link",
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
		{
			spec:     layersPathSpec{name: "foo/bar"},
			expected: "/docker/registry/v2/repositories/foo/bar/_layers",
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
	badpath, err := pathFor(manifestRevisionPathSpec{
		name: "foo/bar",
	})

	if err == nil {
		t.Fatalf("expected an error when mapping an invalid revision: %s", badpath)
	}
}

func TestDigestFromPath(t *testing.T) {
	for _, testcase := range []struct {
		path       string
		expected   digest.Digest
		multilevel bool
		err        error
	}{
		{
			path:       "/docker/registry/v2/blobs/sha256/99/9943fffae777400c0344c58869c4c2619c329ca3ad4df540feda74d291dd7c86/data",
			multilevel: true,
			expected:   "sha256:9943fffae777400c0344c58869c4c2619c329ca3ad4df540feda74d291dd7c86",
			err:        nil,
		},
	} {
		result, err := digestFromPath(testcase.path)
		if err != testcase.err {
			t.Fatalf("Unexpected error value %v when we wanted %v", err, testcase.err)
		}

		if result != testcase.expected {
			t.Fatalf("Unexpected result value %v when we wanted %v", result, testcase.expected)
		}
	}
}

func TestExportedPathHelpers(t *testing.T) {
	const repo = "foo/bar"
	dgst := digest.Digest("sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")

	got, err := BlobLinkPath(repo, dgst)
	if err != nil {
		t.Fatalf("BlobLinkPath: %v", err)
	}
	want, err := pathFor(layerLinkPathSpec{name: repo, digest: dgst})
	if err != nil {
		t.Fatalf("pathFor(layerLinkPathSpec): %v", err)
	}
	if got != want {
		t.Errorf("BlobLinkPath = %q; want %q", got, want)
	}

	got, err = ManifestRevisionLinkPath(repo, dgst)
	if err != nil {
		t.Fatalf("ManifestRevisionLinkPath: %v", err)
	}
	want, err = pathFor(manifestRevisionLinkPathSpec{name: repo, revision: dgst})
	if err != nil {
		t.Fatalf("pathFor(manifestRevisionLinkPathSpec): %v", err)
	}
	if got != want {
		t.Errorf("ManifestRevisionLinkPath = %q; want %q", got, want)
	}

	gotRoot := RepositoriesRootPath()
	wantRoot, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		t.Fatalf("pathFor(repositoriesRootPathSpec): %v", err)
	}
	if gotRoot != wantRoot {
		t.Errorf("RepositoriesRootPath = %q; want %q", gotRoot, wantRoot)
	}

	// Invalid digest should surface as an error rather than a bogus path.
	if _, err := BlobLinkPath(repo, digest.Digest("not-a-digest")); err == nil {
		t.Error("BlobLinkPath: want error for invalid digest, got nil")
	}
	if _, err := ManifestRevisionLinkPath(repo, digest.Digest("not-a-digest")); err == nil {
		t.Error("ManifestRevisionLinkPath: want error for invalid digest, got nil")
	}
}
