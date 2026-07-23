package storage

import (
	"context"
	"io"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

// s3OrderDriver replays the S3 storage driver's Walk semantics: it enumerates
// object keys in plain lexicographic BYTE order (where '/' == 0x2F), infers
// directories as keys stream past (directoryDiff), honours the StartAfter hint
// the same way S3's ListObjectsV2 does (strictly-greater-than), and handles
// ErrSkipDir/ErrFilledBuffer.
//
// This is the one behaviour that distinguishes S3 from the filesystem/inmemory
// WalkFallback (which walks in component-wise order).
type s3OrderDriver struct {
	driver.StorageDriver
	keys []string // full object keys (leaf files), sorted in byte order
}

func newS3OrderDriver(keys []string) *s3OrderDriver {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return &s3OrderDriver{StorageDriver: inmemory.New(), keys: sorted}
}

func (d *s3OrderDriver) Walk(ctx context.Context, from string, f driver.WalkFn, options ...func(*driver.WalkOptions)) error {
	opts := &driver.WalkOptions{}
	for _, o := range options {
		o(opts)
	}
	startAfter := opts.StartAfterHint

	prefix := from
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
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
		// S3 StartAfter returns only keys strictly greater than the hint.
		if startAfter != "" && key <= startAfter {
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

// directoryDiffLocal mirrors the s3-aws driver's directoryDiff.
func directoryDiffLocal(prev, current string) []string {
	var paths []string
	if prev == "" || current == "" {
		return paths
	}
	parent := current
	for {
		parent = path.Dir(parent)
		if parent == "/" || parent == prev || strings.HasPrefix(prev+"/", parent+"/") {
			break
		}
		paths = append(paths, parent)
	}
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	return paths
}

// isSubpathLocal mirrors the s3-aws driver's isSubpath.
func isSubpathLocal(p, parent string) bool {
	if parent == "" {
		return false
	}
	if p == parent {
		return true
	}
	if parent == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, parent+"/")
}

// TestCatalogPaginationLexicalPrefixS3Order is a regression test for the bug
// where _catalog pagination drops a repository whose name is a lexical prefix
// of another repository's name when the backing store enumerates in byte order
// (as the S3 driver does).
func TestCatalogPaginationLexicalPrefixS3Order(t *testing.T) {
	ctx := context.Background()

	repos := []string{
		"simcore/services/dynamic/jupyter-octave-python-math",
		"simcore/services/dynamic/jupyter-octave-python-math-voila",
		"simcore/services/dynamic/jupyter-pysonic",
	}

	keys := make([]string, 0, len(repos))
	for _, r := range repos {
		mp, err := pathFor(manifestsPathSpec{name: r})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, mp+"/revisions/sha256/0000000000000000000000000000000000000000000000000000000000000000/link")
	}

	d := newS3OrderDriver(keys)
	registry, err := NewRegistry(ctx, d, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	var got []string
	last := ""
	for i := 0; i < 100; i++ { // bounded to avoid hanging on a faulty fix
		p := make([]string, 1)
		n, err := registry.Repositories(ctx, p, last)
		got = append(got, p[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Repositories returned error: %v", err)
		}
		if n == 0 {
			break
		}
		last = p[n-1]
	}

	seen := make(map[string]int, len(got))
	for _, r := range got {
		seen[r]++
	}
	for _, want := range repos {
		switch seen[want] {
		case 0:
			t.Errorf("repository %q was dropped from paginated catalog (got %v)", want, got)
		case 1:
			// ok
		default:
			t.Errorf("repository %q was returned %d times (duplicated) (got %v)", want, seen[want], got)
		}
	}
}
