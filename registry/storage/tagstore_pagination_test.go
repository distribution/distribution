package storage

import (
	"context"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
)

// gcsOrderDriver replays the GCS storage driver's Walk semantics: it
// enumerates object keys in plain lexicographic BYTE order (where '/' ==
// 0x2F), infers directories as keys stream past (directoryDiffLocal), and
// honours the StartAfterHint the way the fixed GCS driver does - as the
// byte-successor of the whole subtree already walked, checked against GCS's
// own *inclusive* StartOffset bound (byteSuccessorLocal). See the StartOffset
// comment in registry/storage/driver/gcs/gcs.go's doWalk for why this,
// rather than the bare boundary path, is required.
//
// This is the one behaviour that distinguishes GCS (and S3) from the
// filesystem/inmemory WalkFallback (which walks in component-wise order).
type gcsOrderDriver struct {
	driver.StorageDriver
	keys []string // full object keys (leaf files), sorted in byte order
}

func newGCSOrderDriver(keys []string) *gcsOrderDriver {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return &gcsOrderDriver{StorageDriver: inmemory.New(), keys: sorted}
}

func (d *gcsOrderDriver) Walk(ctx context.Context, from string, f driver.WalkFn, options ...func(*driver.WalkOptions)) error {
	opts := &driver.WalkOptions{}
	for _, o := range options {
		o(opts)
	}
	startAfter := opts.StartAfterHint

	prefix := from
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var startOffset string
	if startAfter != "" {
		subtree := strings.TrimSuffix(startAfter, "/") + "/"
		startOffset = byteSuccessorLocal(subtree)
	}

	prevDir := from
	var prevSkipDir string

	emit := func(fi driver.FileInfo) (stop bool, err error) {
		if isSubpathLocal(fi.Path(), prevSkipDir) {
			return false, nil
		}
		switch werr := f(fi); werr {
		case nil:
			return false, nil
		case driver.ErrSkipDir:
			prevSkipDir = fi.Path()
			return false, nil
		case driver.ErrFilledBuffer:
			return true, nil
		default:
			return true, werr
		}
	}

	for _, key := range d.keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		// GCS StartOffset is an INCLUSIVE bound: keys >= startOffset are
		// returned. Using anything less specific than the byte-successor
		// of the already-walked subtree re-matches a lexically-earlier
		// sibling forever - that's the bug this test guards against.
		if startOffset != "" && key < startOffset {
			continue
		}

		for _, dir := range directoryDiffLocal(prevDir, key) {
			prevDir = dir
			stop, err := emit(driver.FileInfoInternal{FileInfoFields: driver.FileInfoFields{IsDir: true, Path: dir}})
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}

		stop, err := emit(driver.FileInfoInternal{FileInfoFields: driver.FileInfoFields{IsDir: false, Path: key}})
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

// byteSuccessorLocal mirrors the GCS driver's byteSuccessor.
func byteSuccessorLocal(s string) string {
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xff {
			b[i]++
			return string(b[:i+1])
		}
	}
	return s + "\xff"
}

// TestTagListPaginationLexicalPrefixGCSOrder is a regression test for the bug
// where tag-listing pagination bounces between a tag and a sibling whose name
// it's a lexical prefix of (e.g. "1.0" / "1.0.1") when the backing store
// enumerates in byte order (as the GCS driver does): "1.0" is a byte-prefix
// of "1.0.1"'s object key, so using it as the raw pagination cursor makes
// GCS's inclusive StartOffset re-match the sibling on every later page
// instead of skipping past it.
func TestTagListPaginationLexicalPrefixGCSOrder(t *testing.T) {
	ctx := context.Background()
	repoName := "myapp"

	tags := []string{"1.0", "1.0.1", "2.0"}

	keys := make([]string, 0, len(tags))
	for _, tag := range tags {
		p, err := pathFor(manifestTagCurrentPathSpec{name: repoName, tag: tag})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, p)
	}

	d := newGCSOrderDriver(keys)
	reg, err := NewRegistry(ctx, d, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	named, err := reference.WithName(repoName)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reg.Repository(ctx, named)
	if err != nil {
		t.Fatalf("error constructing repository: %v", err)
	}

	ts, ok := repo.Tags(ctx).(*tagStore)
	if !ok {
		t.Fatalf("expected *tagStore, got %T", repo.Tags(ctx))
	}

	var got []string
	last := ""
	for i := 0; i < 100; i++ { // bounded to avoid hanging on a faulty fix
		page, err := ts.List(ctx, 1, last)
		got = append(got, page...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(page) == 0 {
			break
		}
		last = page[len(page)-1]
	}

	seen := make(map[string]int, len(got))
	for _, tag := range got {
		seen[tag]++
	}
	for _, want := range tags {
		switch seen[want] {
		case 0:
			t.Errorf("tag %q was dropped from paginated listing (got %v)", want, got)
		case 1:
			// ok
		default:
			t.Errorf("tag %q was returned %d times (duplicated/looped) (got %v)", want, seen[want], got)
		}
	}
}
