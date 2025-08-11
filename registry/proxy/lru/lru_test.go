package lru

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
)

var testData = [64]byte{}

// testRefPath gives just the digest as a string
func testRefPath(r reference.Reference) string {
	return "/" + strings.Split(r.String(), ":")[1]
}

func testRefs(t *testing.T) (reference.Canonical, reference.Canonical, reference.Canonical) {
	ref1, err := reference.Parse("testrepo@sha256:aaaaeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}
	can1, ok := ref1.(reference.Canonical)
	if !ok {
		t.Fatalf("could not parse canonical reference: %v", ref1)
	}

	ref2, err := reference.Parse("testrepo@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}
	can2, ok := ref2.(reference.Canonical)
	if !ok {
		t.Fatalf("could not parse canonical reference: %v", ref2)
	}

	ref3, err := reference.Parse("testrepo@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}
	can3, ok := ref3.(reference.Canonical)
	if !ok {
		t.Fatalf("could not parse canonical reference: %v", ref3)
	}

	return can1, can2, can3
}

func testRefDup(t *testing.T) reference.Canonical {
	ref, err := reference.Parse("testrepodup@sha256:aaaaeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}
	can, ok := ref.(reference.Canonical)
	if !ok {
		t.Fatalf("could not parse canonical reference: %v", ref)
	}
	return can
}

func testLRU(t *testing.T, driver driver.StorageDriver, path string, options map[string]interface{}) *LRUEvictionController {
	ec, err := new(dcontext.Background(), driver, path, options)
	if err != nil {
		t.Fatalf("Could not create a valid EvictionController, %v", err)
	}
	s, ok := ec.(*LRUEvictionController)
	if !ok {
		t.Fatalf("Could not create a valid LRUEvictionController")
	}
	return s
}

func TestNewLRU(t *testing.T) {
	options := map[string]interface{}{
		"limit": "127",
	}
	testLRU(t, inmemory.New(), "/lru", options)
}

func TestEviction(t *testing.T) {
	ref1, ref2, ref3 := testRefs(t)
	expectedEvictions := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}

	fs := inmemory.New()
	ctx := context.Background()
	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": "127"})
	deleteFunc := func(r reference.Reference) error {
		if len(expectedEvictions) == 0 {
			t.Fatal("Incorrect eviction count")
		}
		_, ok := expectedEvictions[r.String()]
		if !ok {
			t.Fatalf("Trying to remove unexpected repo: %s", r)
		}
		t.Log("removing", r)
		delete(expectedEvictions, r.String())
		if err := fs.Delete(ctx, testRefPath(r)); err != nil {
			t.Fatalf("Error while deleting file %s: %v", testRefPath(r), err)
		}

		return nil
	}
	ec.onBlobExpire = deleteFunc

	// Instead of calling Start, we set stopped = false instead so the entries will not be written to fs
	// This is a workaround to make this test not flaky, as the entries are written every 5 second, which
	// affects the size/limit calculations
	ec.stopped = false

	if err := fs.PutContent(ctx, testRefPath(ref1), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := ec.add(ref1, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref1, err)
	}

	if err := fs.PutContent(ctx, testRefPath(ref2), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := ec.add(ref2, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref2, err)
	}

	if err := fs.PutContent(ctx, testRefPath(ref3), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := ec.add(ref3, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref3, err)
	}

	if len(expectedEvictions) != 0 {
		t.Fatalf("Repositories remaining: %#v", expectedEvictions)
	}
}

func TestRestoreOld(t *testing.T) {
	ref1, ref2, ref3 := testRefs(t)
	expectedEvictions := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}

	fs := inmemory.New()
	ctx := dcontext.Background()
	deleteFunc := func(r reference.Reference) error {
		if len(expectedEvictions) == 0 {
			t.Fatal("Incorrect eviction count")
		}
		_, ok := expectedEvictions[r.String()]
		if !ok {
			t.Fatalf("Trying to remove unexpected repo: %s", r)
		}
		t.Log("removing", r)
		delete(expectedEvictions, r.String())
		if err := fs.Delete(ctx, testRefPath(r)); err != nil {
			t.Fatalf("Error while deleting file %s: %v", testRefPath(r), err)
		}

		return nil
	}

	serialized, err := json.Marshal(&map[string]lruEntry{
		entryNameHead: {
			Key:       entryNameHead,
			EntryType: 0,
			Prev:      entryNameHead,
			Next:      ref1.Digest().String(),
		},
		entryNameTail: {
			Key:       entryNameTail,
			EntryType: 0,
			Prev:      ref2.Digest().String(),
			Next:      entryNameTail,
		},
		ref1.Digest().String(): {
			Key:        ref1.Digest().String(),
			EntryType:  0,
			References: []string{ref1.Name()},
			Prev:       entryNameHead,
			Next:       ref2.Digest().String(),
		},
		ref2.Digest().String(): {
			Key:        ref2.Digest().String(),
			EntryType:  0,
			References: []string{ref2.Name()},
			Prev:       ref1.Digest().String(),
			Next:       entryNameTail,
		},
	})
	if err != nil {
		t.Fatalf("Error serializing test data: %s", err.Error())
	}

	pathToStatFile := "/lru"
	err = fs.PutContent(ctx, pathToStatFile, serialized)
	if err != nil {
		t.Fatal("Unable to write serialized data to fs")
	}

	if err := fs.PutContent(ctx, testRefPath(ref1), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := fs.PutContent(ctx, testRefPath(ref2), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}

	// Account for the disk space of the written entries file
	limit := 127 + len(serialized)
	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": strconv.Itoa(limit)})
	ec.OnBlobEvict(deleteFunc)
	err = ec.Start()
	if err != nil {
		t.Fatalf("Error starting LRUEvictionController: %s", err)
	}

	if err := fs.PutContent(ctx, testRefPath(ref3), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := ec.add(ref3, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref3, err)
	}

	if len(expectedEvictions) != 0 {
		t.Fatalf("Repositories remaining: %#v", expectedEvictions)
	}

	if err := ec.Stop(); err != nil {
		t.Fatalf("Error stopping LRUEvictionController: %s", err)
	}
}

func TestStopRestore(t *testing.T) {
	ref1, ref2, _ := testRefs(t)

	deleteFunc := func(r reference.Reference) error {
		t.Fatalf("No deletion expected, trying to delete %s", r.String())
		return nil
	}

	fs := inmemory.New()
	pathToStateFile := "/lru"
	ec := testLRU(t, fs, pathToStateFile, map[string]interface{}{"limit": "127"})
	ec.onBlobExpire = deleteFunc

	err := ec.Start()
	if err != nil {
		t.Fatal(err)
	}
	if err := ec.add(ref1, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref1, err)
	}
	if err := ec.add(ref2, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref2, err)
	}

	// Start and stop before all operations complete
	// state will be written to fs
	err = ec.Stop()
	if err != nil {
		t.Fatal(err)
	}

	// ec2 will restore state from fs
	ec2 := testLRU(t, fs, pathToStateFile, map[string]interface{}{"limit": "127"})
	ec2.onBlobExpire = deleteFunc
	err = ec2.Start()
	if err != nil {
		t.Fatalf("Error starting v2: %s", err.Error())
	}

	// We added 2 entries, and there's entries for head and tail
	if len(ec2.entries) != 4 {
		t.Fatalf("Unexpected number of restored entries: %v", ec2.entries)
	}
}

func TestDoubleStart(t *testing.T) {
	ec := testLRU(t, inmemory.New(), "/lru", map[string]interface{}{"limit": "127"})
	err := ec.Start()
	if err != nil {
		t.Fatal("Unable to start LRU")
	}
	err = ec.Start()
	if err == nil {
		t.Fatal("LRU started twice without error")
	}
}

func TestEvictEmpty(t *testing.T) {
	fs := inmemory.New()
	ctx := context.Background()

	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": "63"})
	err := ec.Start()
	if err != nil {
		t.Fatal("Unable to start LRU")
	}

	if err := fs.PutContent(ctx, "/testdata1", testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}

	// This case should not happen normally, but is possible if, for example, the limit is set too low
	// and even evicting every entry does not allow for enough space.
	err = ec.evict()
	if err.Error() != "Cache eviction has evicted all entries but storage is still above limit" {
		t.Fatalf("Reached unexpected error %s", err)
	}
}

func TestEvictNothing(t *testing.T) {
	fs := inmemory.New()

	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": "63"})
	err := ec.Start()
	if err != nil {
		t.Fatal("Unable to start LRU")
	}

	err = ec.evict()
	if err != nil {
		t.Fatalf("Reached unexcepted error during eviction: %v", err)
	}
}

func TestEvictTouch(t *testing.T) {
	ref1, ref2, _ := testRefs(t)

	fs := inmemory.New()
	ctx := context.Background()

	expectedEvictions := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}
	deleteFunc := func(r reference.Reference) error {
		if len(expectedEvictions) == 0 {
			t.Fatal("Incorrect eviction count")
		}

		if r.String() == ref1.String() && len(expectedEvictions) == 2 {
			t.Fatalf("ref1 should not be removed first")
		}
		_, ok := expectedEvictions[r.String()]
		if !ok {
			t.Fatalf("Trying to remove unexpected repo: %s", r)
		}
		t.Log("removing", r)
		delete(expectedEvictions, r.String())
		if err := fs.Delete(ctx, testRefPath(r)); err != nil {
			t.Fatalf("Error while deleting file %s: %v", testRefPath(r), err)
		}

		return nil
	}

	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": "63"})
	ec.OnBlobEvict(deleteFunc)
	ec.stopped = false

	if err := ec.add(ref1, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref1, err)
	}
	if err := ec.add(ref2, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref2, err)
	}

	if err := fs.PutContent(ctx, testRefPath(ref1), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := fs.PutContent(ctx, testRefPath(ref2), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}

	if err := ec.touch(ref1, entryTypeBlob); err != nil {
		t.Fatalf("Failed to touch entry %s in LRUEvictionController: %v", ref1, err)
	}

	err := ec.evict()
	if err != nil {
		t.Fatalf("Reached unexcepted error during eviction: %v", err)
	}
}

func TestDuplicate(t *testing.T) {
	ref1, _, _ := testRefs(t)
	ref2 := testRefDup(t)

	fs := inmemory.New()
	ctx := context.Background()

	expectedEvictions := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}
	deleteFunc := func(r reference.Reference) error {
		if len(expectedEvictions) == 0 {
			t.Fatal("Incorrect eviction count")
		}
		_, ok := expectedEvictions[r.String()]
		if !ok {
			t.Fatalf("Trying to remove unexpected repo: %s", r)
		}
		t.Log("removing", r)

		delete(expectedEvictions, r.String())
		if err := fs.Delete(ctx, testRefPath(r)); err != nil {
			if errors.Is(err, driver.PathNotFoundError{Path: testRefPath(ref2), DriverName: "inmemory"}) {
				// the second delete will try to remove the same file
				return nil
			}
			t.Fatalf("Error while deleting file %s: %v", testRefPath(r), err)
		}

		return nil
	}

	ec := testLRU(t, fs, "/lru", map[string]interface{}{"limit": "63"})
	ec.OnBlobEvict(deleteFunc)
	ec.stopped = false

	if err := ec.add(ref1, entryTypeBlob); err != nil {
		t.Fatalf("Failed to add entry %s to LRUEvictionController: %v", ref1, err)
	}
	if err := ec.touch(ref2, entryTypeBlob); err != nil {
		t.Fatalf("Failed to touch entry %s in LRUEvictionController: %v", ref2, err)
	}

	// HEAD, TAIL, and the deduplicated entry of ref1 and ref2
	if len(ec.entries) != 3 {
		t.Fatalf("Entry add did not deduplicate entries with the same digest")
	}

	// testRefPath for ref1 and ref2 should be the same, so this will write a total of 64 bytes
	if err := fs.PutContent(ctx, testRefPath(ref1), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}
	if err := fs.PutContent(ctx, testRefPath(ref2), testData[:]); err != nil {
		t.Fatalf("Unable to write test data to fs: %v", err)
	}

	err := ec.evict()
	if err != nil {
		t.Fatalf("Reached unexcepted error during eviction: %v", err)
	}

	if len(expectedEvictions) != 0 {
		t.Fatalf("Did not evict all references of a duplicated digest entry, left with %v", expectedEvictions)
	}
}
