package storage

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestVerifyOCIManifestNonDistributableLayer(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()
	registry := createRegistry(t, inmemoryDriver,
		ManifestURLsAllowRegexp(regexp.MustCompile("^https?://foo")),
		ManifestURLsDenyRegexp(regexp.MustCompile("^https?://foo/nope")))
	repo := makeRepository(t, registry, "test")
	manifestService := makeManifestService(t, repo)

	config, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	layer, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayerGzip, nil)
	if err != nil {
		t.Fatal(err)
	}

	nonDistributableLayer := v1.Descriptor{
		Digest:    "sha256:463435349086340864309863409683460843608348608934092322395278926a",
		Size:      6323,
		MediaType: v1.MediaTypeImageLayerNonDistributableGzip, //nolint:staticcheck // ignore A1019: v1.MediaTypeImageLayerNonDistributableGzip is deprecated: Non-distributable layers are deprecated, and not recommended for future use
	}

	emptyLayer := v1.Descriptor{
		Digest: "",
	}

	emptyGzipLayer := v1.Descriptor{
		Digest:    "",
		MediaType: v1.MediaTypeImageLayerGzip,
	}

	template := ocischema.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: v1.MediaTypeImageManifest,
		Config:    config,
	}

	type testcase struct {
		BaseLayer v1.Descriptor
		URLs      []string
		Err       error
	}

	cases := []testcase{
		{
			nonDistributableLayer,
			nil,
			distribution.ErrManifestBlobUnknown{Digest: nonDistributableLayer.Digest},
		},
		{
			layer,
			[]string{"http://foo/bar"},
			nil,
		},
		{
			nonDistributableLayer,
			[]string{"file:///local/file"},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"http://foo/bar#baz"},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{""},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"https://foo/bar", ""},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"", "https://foo/bar"},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"http://nope/bar"},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"http://foo/nope"},
			errInvalidURL,
		},
		{
			nonDistributableLayer,
			[]string{"http://foo/bar"},
			nil,
		},
		{
			nonDistributableLayer,
			[]string{"https://foo/bar"},
			nil,
		},
		{
			emptyLayer,
			[]string{"https://foo/empty"},
			digest.ErrDigestInvalidFormat,
		},
		{
			emptyLayer,
			[]string{},
			digest.ErrDigestInvalidFormat,
		},
		{
			emptyGzipLayer,
			[]string{"https://foo/empty"},
			digest.ErrDigestInvalidFormat,
		},
		{
			emptyGzipLayer,
			[]string{},
			digest.ErrDigestInvalidFormat,
		},
	}

	for _, c := range cases {
		m := template
		l := c.BaseLayer
		l.URLs = c.URLs
		m.Layers = []v1.Descriptor{l}
		dm, err := ocischema.FromStruct(m)
		if err != nil {
			t.Error(err)
			continue
		}

		_, err = manifestService.Put(ctx, dm)
		if verr, ok := err.(distribution.ErrManifestVerification); ok {
			// Extract the first error
			if len(verr) == 2 {
				if _, ok = verr[1].(distribution.ErrManifestBlobUnknown); ok {
					err = verr[0]
				}
			} else if len(verr) == 1 {
				err = verr[0]
			}
		}
		if err != c.Err {
			t.Errorf("%#v: expected %v, got %v", l, c.Err, err)
		}
	}
}

func TestVerifyOCIManifestBlobLayerAndConfig(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()
	registry := createRegistry(t, inmemoryDriver,
		ManifestURLsAllowRegexp(regexp.MustCompile("^https?://foo")),
		ManifestURLsDenyRegexp(regexp.MustCompile("^https?://foo/nope")))

	repo := makeRepository(t, registry, strings.ToLower(t.Name()))
	manifestService := makeManifestService(t, repo)

	config, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	layer, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayerGzip, nil)
	if err != nil {
		t.Fatal(err)
	}

	template := ocischema.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: v1.MediaTypeImageManifest,
	}

	checkFn := func(m ocischema.Manifest, rerr error) {
		dm, err := ocischema.FromStruct(m)
		if err != nil {
			t.Error(err)
			return
		}

		_, err = manifestService.Put(ctx, dm)
		if verr, ok := err.(distribution.ErrManifestVerification); ok {
			// Extract the first error
			if len(verr) == 2 {
				if _, ok = verr[1].(distribution.ErrManifestBlobUnknown); ok {
					err = verr[0]
				}
			} else if len(verr) == 1 {
				err = verr[0]
			}
		}
		if err != rerr {
			t.Errorf("%#v: expected %v, got %v", m, rerr, err)
		}
	}

	type testcase struct {
		Desc v1.Descriptor
		URLs []string
		Err  error
	}

	layercases := []testcase{
		// empty media type
		{
			v1.Descriptor{},
			[]string{"http://foo/bar"},
			digest.ErrDigestInvalidFormat,
		},
		{
			v1.Descriptor{},
			nil,
			digest.ErrDigestInvalidFormat,
		},
		// unknown media type, but blob is present
		{
			v1.Descriptor{
				Digest: layer.Digest,
			},
			nil,
			nil,
		},
		{
			v1.Descriptor{
				Digest: layer.Digest,
			},
			[]string{"http://foo/bar"},
			nil,
		},
		// gzip layer, but invalid digest
		{
			v1.Descriptor{
				MediaType: v1.MediaTypeImageLayerGzip,
			},
			nil,
			digest.ErrDigestInvalidFormat,
		},
		{
			v1.Descriptor{
				MediaType: v1.MediaTypeImageLayerGzip,
			},
			[]string{"https://foo/bar"},
			digest.ErrDigestInvalidFormat,
		},
		{
			v1.Descriptor{
				MediaType: v1.MediaTypeImageLayerGzip,
				Digest:    digest.Digest("invalid"),
			},
			nil,
			digest.ErrDigestInvalidFormat,
		},
		// normal uploaded gzip layer
		{
			layer,
			nil,
			nil,
		},
		{
			layer,
			[]string{"https://foo/bar"},
			nil,
		},
	}

	for _, c := range layercases {
		m := template
		m.Config = config

		l := c.Desc
		l.URLs = c.URLs

		m.Layers = []v1.Descriptor{l}

		checkFn(m, c.Err)
	}

	configcases := []testcase{
		// valid config
		{
			config,
			nil,
			nil,
		},
		// invalid digest
		{
			v1.Descriptor{
				MediaType: v1.MediaTypeImageConfig,
			},
			[]string{"https://foo/bar"},
			digest.ErrDigestInvalidFormat,
		},
		{
			v1.Descriptor{
				MediaType: v1.MediaTypeImageConfig,
				Digest:    digest.Digest("invalid"),
			},
			nil,
			digest.ErrDigestInvalidFormat,
		},
	}

	for _, c := range configcases {
		m := template
		m.Config = c.Desc
		m.Config.URLs = c.URLs

		checkFn(m, c.Err)
	}
}
