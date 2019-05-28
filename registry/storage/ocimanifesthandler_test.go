package storage

import (
	"context"
	"regexp"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/manifest/ocischema"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/opencontainers/image-spec/specs-go/v1"
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

	nonDistributableLayer := distribution.Descriptor{
		Digest:    "sha256:463435349086340864309863409683460843608348608934092322395278926a",
		Size:      6323,
		MediaType: v1.MediaTypeImageLayerNonDistributableGzip,
	}

	template := ocischema.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     v1.MediaTypeImageManifest,
		},
		Config: config,
	}

	type testcase struct {
		BaseLayer distribution.Descriptor
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
	}

	for _, c := range cases {
		m := template
		l := c.BaseLayer
		l.URLs = c.URLs
		m.Layers = []distribution.Descriptor{l}
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

func TestVerifyOCIManifestForbiddenMediaTypes(t *testing.T) {
	testForbiddenMediaTypes(t, true)
}

// this is a helper which allows us to run the same exact test cases
// against both ocischema.Manifest and schema2.Manifest
func testForbiddenMediaTypes(t *testing.T, isOCI bool) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()
	registry := createRegistry(t, inmemoryDriver,
		ManifestConfigMediaTypesAllowRegexp(regexp.MustCompile(`^application/vnd\.fruits\.(banana|orange)\.config.*$`)),
		ManifestConfigMediaTypesDenyRegexp(regexp.MustCompile("^.*[.+]yaml$")),
		ManifestLayerMediaTypesAllowRegexp(regexp.MustCompile(`^application/vnd\.fruits\.(banana|orange)\.(peel|edible)\.layer.*$`)),
		ManifestLayerMediaTypesDenyRegexp(regexp.MustCompile("^.*[.+]yaml$")))

	repo := makeRepository(t, registry, "test")
	manifestService := makeManifestService(t, repo)

	config, err := repo.Blobs(ctx).Put(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	layer0, err := repo.Blobs(ctx).Put(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	layer1, err := repo.Blobs(ctx).Put(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	layers := []distribution.Descriptor{layer0, layer1}
	versioned := manifest.Versioned{
		SchemaVersion: 2,
	}

	var ocischemaManifest ocischema.Manifest
	var schema2Manifest schema2.Manifest

	if isOCI {
		ocischemaManifest = ocischema.Manifest{
			Versioned: versioned,
			Config:    config,
			Layers:    layers,
		}
	} else {
		schema2Manifest = schema2.Manifest{
			Versioned: versioned,
			Config:    config,
			Layers:    layers,
		}
	}

	type (
		mediaTypeTestError string
	)

	const (
		noError                 mediaTypeTestError = "nil"
		badConfigMediaTypeError mediaTypeTestError = "ErrManifestConfigMediaTypeForbidden"
		badLayerMediaTypeError  mediaTypeTestError = "ErrManifestLayerMediaTypeForbidden"
	)

	type mediaTypeTestCase struct {
		configMediaType   string
		layer0MediaType   string
		layer1MediaType   string
		expectedErrorType mediaTypeTestError
	}

	allMediaTypeTestCases := []mediaTypeTestCase{
		{
			"application/vnd.fruits.banana.config.v1+json",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			noError,
		},
		{
			"application/vnd.fruits.orange.config.v1+json",
			"application/vnd.fruits.orange.peel.layer.v1+tar",
			"application/vnd.fruits.orange.edible.layer.v1+tar",
			noError,
		},
		{
			"application/vnd.fruits.orange.config.v1+json",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			noError,
		},
		{
			"application/vnd.fruits.banana.config.v1+json",
			"application/vnd.fruits.orange.peel.layer.v1+tar",
			"application/vnd.fruits.orange.edible.layer.v1+tar",
			noError,
		},
		{
			"application/vnd.fruits.banana.config.v1+txt",
			"application/vnd.fruits.banana.peel.layer.v1+json",
			"application/vnd.fruits.banana.edible.layer.v1+json",
			noError,
		},
		{
			"application/vnd.fruits.banana.config.v1+txt",
			"application/vnd.fruits.orange.peel.layer.v1+json",
			"application/vnd.fruits.banana.edible.layer.v1+json",
			noError,
		},
		{
			"application/vnd.fruits.kiwi.config.v1+json",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			badConfigMediaTypeError,
		},
		{
			"application/vnd.fruits.banana.config.v1+yaml",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			badConfigMediaTypeError,
		},
		{
			"application/vnd.fruits.banana.config.v1.yaml",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			badConfigMediaTypeError,
		},
		{
			"application/vnd.fruits.banana.confiiiiiig.v1+json",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1+tar",
			badConfigMediaTypeError,
		},
		{
			"application/vnd.fruits.orange.config.v1+json",
			"application/vnd.fruits.kiwi.peel.layer.v1+tar",
			"application/vnd.fruits.orange.edible.layer.v1+tar",
			badLayerMediaTypeError,
		},
		{
			"application/vnd.fruits.orange.config.v1+json",
			"application/vnd.fruits.orange.peel.layer.v1+tar",
			"application/vnd.fruits.kiwi.edible.layer.v1+tar",
			badLayerMediaTypeError,
		},
		{
			"application/vnd.fruits.orange.config.v1+json",
			"application/vnd.fruits.orange.peel.layer.v1+yaml",
			"application/vnd.fruits.orange.edible.layer.v1+tar",
			badLayerMediaTypeError,
		},
		{
			"application/vnd.fruits.banana.config.v1+json",
			"application/vnd.fruits.banana.peel.layer.v1+tar",
			"application/vnd.fruits.banana.edible.layer.v1.yaml",
			badLayerMediaTypeError,
		},
	}

	for _, c := range allMediaTypeTestCases {
		var dm distribution.Manifest
		var err error
		if isOCI {
			ocischemaManifest.Config.MediaType = c.configMediaType
			ocischemaManifest.Layers[0].MediaType = c.layer0MediaType
			ocischemaManifest.Layers[1].MediaType = c.layer1MediaType
			dm, err = ocischema.FromStruct(ocischemaManifest)
		} else {
			schema2Manifest.Config.MediaType = c.configMediaType
			schema2Manifest.Layers[0].MediaType = c.layer0MediaType
			schema2Manifest.Layers[1].MediaType = c.layer1MediaType
			dm, err = schema2.FromStruct(schema2Manifest)
		}

		if err != nil {
			t.Fatal(err)
		}

		_, err = manifestService.Put(ctx, dm)

		if err == nil {
			if c.expectedErrorType != noError {
				t.Errorf("expected error %s but instead got nil", c.expectedErrorType)
			}
		} else {
			switch err.(type) {
			case distribution.ErrManifestConfigMediaTypeForbidden:
				if c.expectedErrorType != badConfigMediaTypeError {
					t.Errorf("expected error %s but instead got error: %s", badConfigMediaTypeError, err)
				}
			case distribution.ErrManifestLayerMediaTypeForbidden:
				if c.expectedErrorType != badLayerMediaTypeError {
					t.Errorf("expected error %s but instead got error: %s", badConfigMediaTypeError, err)
				}
			default:
				t.Errorf("unknown error: %s", err)
			}
		}
	}
}
