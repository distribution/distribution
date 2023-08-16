package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/distribution"
	digest "github.com/opencontainers/go-digest"
)

func TestCacheSet(t *testing.T) {
	cache := newTestStatter()
	backend := newTestStatter()
	st := NewCachedBlobStatter(cache, backend)
	ctx := context.Background()

	dgst := digest.Digest("dontvalidate")
	_, err := st.Stat(ctx, dgst)
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("Unexpected error %v, expected %v", err, distribution.ErrBlobUnknown)
	}

	desc := distribution.Descriptor{
		Digest: dgst,
	}
	if err := backend.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatal(err)
	}

	actual, err := st.Stat(ctx, dgst)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != desc.Digest {
		t.Fatalf("Unexpected descriptor %v, expected %v", actual, desc)
	}

	if len(cache.sets) != 1 || len(cache.sets[dgst]) == 0 {
		t.Fatalf("Expected cache set")
	}
	if cache.sets[dgst][0].Digest != desc.Digest {
		t.Fatalf("Unexpected descriptor %v, expected %v", cache.sets[dgst][0], desc)
	}

	desc2 := distribution.Descriptor{
		Digest: digest.Digest("dontvalidate 2"),
	}
	cache.sets[dgst] = append(cache.sets[dgst], desc2)

	actual, err = st.Stat(ctx, dgst)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != desc2.Digest {
		t.Fatalf("Unexpected descriptor %v, expected %v", actual, desc)
	}
}

func TestCacheError(t *testing.T) {
	cache := newErrTestStatter(errors.New("cache error"))
	backend := newTestStatter()
	st := NewCachedBlobStatter(cache, backend)
	ctx := context.Background()

	dgst := digest.Digest("dontvalidate")
	_, err := st.Stat(ctx, dgst)
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("Unexpected error %v, expected %v", err, distribution.ErrBlobUnknown)
	}

	desc := distribution.Descriptor{
		Digest: dgst,
	}
	if err := backend.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatal(err)
	}

	actual, err := st.Stat(ctx, dgst)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != desc.Digest {
		t.Fatalf("Unexpected descriptor %v, expected %v", actual, desc)
	}

	if len(cache.sets) > 0 {
		t.Fatalf("Set should not be called after stat error")
	}
}

func newTestStatter() *testStatter {
	return &testStatter{
		stats: []digest.Digest{},
		sets:  map[digest.Digest][]distribution.Descriptor{},
	}
}

func newErrTestStatter(err error) *testStatter {
	return &testStatter{
		sets: map[digest.Digest][]distribution.Descriptor{},
		err:  err,
	}
}

type testStatter struct {
	stats []digest.Digest
	sets  map[digest.Digest][]distribution.Descriptor
	err   error
}

func (s *testStatter) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	if s.err != nil {
		return distribution.Descriptor{}, s.err
	}

	if set := s.sets[dgst]; len(set) > 0 {
		return set[len(set)-1], nil
	}

	return distribution.Descriptor{}, distribution.ErrBlobUnknown
}

func (s *testStatter) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	s.sets[dgst] = append(s.sets[dgst], desc)
	return s.err
}

func (s *testStatter) Clear(ctx context.Context, dgst digest.Digest) error {
	return s.err
}
