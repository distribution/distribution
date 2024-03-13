package scheduler

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
)

func testRefs(t *testing.T) (reference.Canonical, reference.Canonical, reference.Canonical) {
	ref1, err := reference.Parse("testrepo@sha256:aaaaeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}

	ref2, err := reference.Parse("testrepo@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}

	ref3, err := reference.Parse("testrepo@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}

	return ref1.(reference.Canonical), ref2.(reference.Canonical), ref3.(reference.Canonical)
}

func TestSchedule(t *testing.T) {
	ref1, ref2, ref3 := testRefs(t)
	timeUnit := time.Millisecond
	remainingRefs := map[string]bool{
		ref1.Digest().String(): true,
		ref2.Digest().String(): true,
		ref3.Digest().String(): true,
	}

	var mu sync.Mutex
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	deleteFunc := func(ref reference.Reference) error {
		cRef, ok := ref.(reference.Canonical)
		if !ok {
			t.Fatalf("reference is not cannonical (includes name & digest): %v", ref)
		}
		refKey := cRef.Digest().String()

		if len(remainingRefs) == 0 {
			t.Fatalf("Incorrect expiry count")
		}
		_, ok = remainingRefs[refKey]
		if !ok {
			t.Fatalf("Trying to remove nonexistent ref: %s", refKey)
		}
		t.Log("removing", refKey)
		mu.Lock()
		delete(remainingRefs, refKey)
		mu.Unlock()

		return nil
	}
	s.onBlobExpire = deleteFunc
	err := s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}

	s.add(ref1, 3*timeUnit, entryTypeBlob)
	s.add(ref2, 1*timeUnit, entryTypeBlob)

	func() {
		s.Lock()
		s.add(ref3, 1*timeUnit, entryTypeBlob)
		s.Unlock()
	}()

	// Ensure all refs are deleted
	<-time.After(50 * timeUnit)

	mu.Lock()
	defer mu.Unlock()
	if len(remainingRefs) != 0 {
		t.Fatalf("Refs remaining: %#v", remainingRefs)
	}
}

func TestRestoreOld(t *testing.T) {
	ref1, ref2, _ := testRefs(t)
	remainingRefs := map[string]bool{
		ref1.Digest().String(): true,
		ref2.Digest().String(): true,
	}

	var wg sync.WaitGroup
	wg.Add(len(remainingRefs))
	var mu sync.Mutex
	deleteFunc := func(ref reference.Reference) error {
		mu.Lock()
		defer mu.Unlock()

		cRef, ok := ref.(reference.Canonical)
		if !ok {
			t.Fatalf("reference is not cannonical (includes name & digest): %v", ref)
		}
		refKey := cRef.Digest().String()

		if cRef.(reference.Canonical).Digest() == ref1.Digest() && len(remainingRefs) == 2 {
			t.Errorf("ref1 should not be removed first")
		}
		_, ok = remainingRefs[refKey]
		if !ok {
			t.Fatalf("Trying to remove nonexistent ref: %s", refKey)
		}
		delete(remainingRefs, refKey)
		wg.Done()
		return nil
	}

	timeUnit := time.Millisecond
	serialized, err := json.Marshal(&map[string]schedulerEntry{
		ref1.Digest().String(): {
			Expiry:    time.Now().Add(10 * timeUnit),
			Key:       ref1.String(),
			EntryType: 0,
		},
		ref2.Digest().String(): {
			Expiry:    time.Now().Add(-3 * timeUnit), // TTL passed, should be removed first
			Key:       ref2.String(),
			EntryType: 0,
		},
	})
	if err != nil {
		t.Fatalf("Error serializing test data: %s", err.Error())
	}

	ctx := dcontext.Background()
	pathToStatFile := "/ttl"
	fs := inmemory.New()
	err = fs.PutContent(ctx, pathToStatFile, serialized)
	if err != nil {
		t.Fatal("Unable to write serialized data to fs")
	}
	s := New(dcontext.Background(), fs, "/ttl")
	s.OnBlobExpire(deleteFunc)
	err = s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}
	defer s.Stop()

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(remainingRefs) != 0 {
		t.Fatalf("Refs remaining: %#v", remainingRefs)
	}
}

func TestStopRestore(t *testing.T) {
	ref1, ref2, _ := testRefs(t)

	timeUnit := time.Millisecond
	remainingRefs := map[string]bool{
		ref1.Digest().String(): true,
		ref2.Digest().String(): true,
	}

	var mu sync.Mutex
	deleteFunc := func(r reference.Reference) error {
		mu.Lock()
		delete(remainingRefs, r.(reference.Canonical).Digest().String())
		mu.Unlock()
		return nil
	}

	fs := inmemory.New()
	pathToStateFile := "/ttl"
	s := New(dcontext.Background(), fs, pathToStateFile)
	s.onBlobExpire = deleteFunc

	err := s.Start()
	if err != nil {
		t.Fatalf(err.Error())
	}
	s.add(ref1, 300*timeUnit, entryTypeBlob)
	s.add(ref2, 100*timeUnit, entryTypeBlob)

	// Start and stop before all operations complete
	// state will be written to fs
	s.Stop()
	time.Sleep(10 * time.Millisecond)

	// v2 will restore state from fs
	s2 := New(dcontext.Background(), fs, pathToStateFile)
	s2.onBlobExpire = deleteFunc
	err = s2.Start()
	if err != nil {
		t.Fatalf("Error starting v2: %s", err.Error())
	}

	<-time.After(500 * timeUnit)
	mu.Lock()
	defer mu.Unlock()
	if len(remainingRefs) != 0 {
		t.Fatalf("Refs remaining: %#v", remainingRefs)
	}
}

func TestDoubleStart(t *testing.T) {
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	err := s.Start()
	if err != nil {
		t.Fatalf("Unable to start scheduler")
	}
	err = s.Start()
	if err == nil {
		t.Fatalf("Scheduler started twice without error")
	}
}

func TestCommonRef(t *testing.T) {
	ref1, ref2, ref3 := testRefs(t)

	timeUnit := time.Millisecond

	// Create a shared blob reference for ref3
	ref3Copy, err := reference.Parse("anothertestrepo@" + ref3.Digest().String())
	if err != nil {
		t.Fatalf("could not parse reference: %v", err)
	}
	cRef3Copy := ref3Copy.(reference.Canonical)

	remainingRefs := map[string]bool{
		ref1.Digest().String(): true,
		ref2.Digest().String(): true,
		ref3.Digest().String(): true,
	}

	var mu sync.Mutex
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	deleteFunc := func(ref reference.Reference) error {
		cRef, ok := ref.(reference.Canonical)
		if !ok {
			t.Fatalf("reference is not cannonical (includes name & digest): %v", ref)
		}
		refKey := cRef.Digest().String()

		if len(remainingRefs) == 0 {
			t.Fatalf("Incorrect expiry count")
		}
		_, ok = remainingRefs[refKey]
		if !ok {
			t.Fatalf("Trying to remove nonexistent ref: %s", refKey)
		}
		t.Log("removing", refKey)
		mu.Lock()
		delete(remainingRefs, refKey)
		mu.Unlock()

		return nil
	}
	s.onBlobExpire = deleteFunc
	err = s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}

	s.add(ref1, 3*timeUnit, entryTypeBlob)
	s.add(ref2, 1*timeUnit, entryTypeBlob)

	func() {
		s.Lock()
		s.add(ref3, 1*timeUnit, entryTypeBlob)
		// This should override the existing expiry of ref3
		s.add(cRef3Copy, 60000*timeUnit, entryTypeBlob)
		s.Unlock()
	}()

	// Wait for refs to be deleted
	<-time.After(50 * timeUnit)

	mu.Lock()
	defer mu.Unlock()

	// Only the common blob should be reminaing
	if len(remainingRefs) != 1 {
		t.Fatalf("Expected 1 ref remaining, but got: %#v", remainingRefs)
	}
	if _, ok := remainingRefs[ref3.Digest().String()]; !ok {
		t.Fatalf("Expected ref3 to be remaining, but got: %#v", remainingRefs)
	}
}
