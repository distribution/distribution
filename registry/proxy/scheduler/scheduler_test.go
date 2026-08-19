package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

func testRefs(t *testing.T) (reference.Reference, reference.Reference, reference.Reference) {
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

	return ref1, ref2, ref3
}

func testRefsN(t *testing.T, n int) []reference.Canonical {
	refs := make([]reference.Canonical, 0, n)
	for i := range n {
		name := "testrepo@sha256:" + fmt.Sprintf("%064d", i)

		ref, err := reference.Parse(name)
		if err != nil {
			t.Fatalf("could not parse reference: %v", err)
		}

		canonical, ok := ref.(reference.Canonical)
		if !ok {
			t.Fatalf("not canonical reference: %v", ref)
		}

		refs = append(refs, canonical)
	}

	return refs
}

func TestSchedule(t *testing.T) {
	refs := testRefsN(t, 20)
	timeUnit := time.Millisecond

	remainingRepos := map[string]bool{}
	for _, ref := range refs {
		remainingRepos[ref.String()] = true
	}

	var mu sync.Mutex
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	deleteFunc := func(repoName reference.Reference) error {
		if len(remainingRepos) == 0 {
			t.Fatal("Incorrect expiry count")
		}
		_, ok := remainingRepos[repoName.String()]
		if !ok {
			t.Fatalf("Trying to remove nonexistent repo: %s", repoName)
		}
		t.Log("removing", repoName)
		mu.Lock()
		delete(remainingRepos, repoName.String())
		mu.Unlock()

		return nil
	}
	s.onBlobExpire = deleteFunc
	err := s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}

	for i, ref := range refs {
		_ = s.AddBlob(ref, time.Duration(i)*timeUnit)
	}

	// Ensure all repos are deleted
	<-time.After(50 * timeUnit)

	mu.Lock()
	defer mu.Unlock()
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}
}

func TestRestoreOld(t *testing.T) {
	ref1, ref2, _ := testRefs(t)
	remainingRepos := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}

	var wg sync.WaitGroup
	wg.Add(len(remainingRepos))
	var mu sync.Mutex
	deleteFunc := func(r reference.Reference) error {
		mu.Lock()
		defer mu.Unlock()
		if r.String() == ref1.String() && len(remainingRepos) == 2 {
			t.Errorf("ref1 should not be removed first")
		}
		_, ok := remainingRepos[r.String()]
		if !ok {
			t.Fatalf("Trying to remove nonexistent repo: %s", r)
		}
		delete(remainingRepos, r.String())
		wg.Done()
		return nil
	}

	timeUnit := time.Millisecond
	serialized, err := json.Marshal(&map[string]schedulerEntry{
		ref1.String(): {
			Expiry:    time.Now().Add(10 * timeUnit),
			Key:       ref1.String(),
			EntryType: 0,
		},
		ref2.String(): {
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
	defer func(s *TTLExpirationScheduler) {
		err := s.Stop()
		if err != nil {
			t.Fatalf("Error stopping ttlExpirationScheduler: %s", err)
		}
	}(s)

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}
}

func TestStopRestore(t *testing.T) {
	ref1, ref2, _ := testRefs(t)

	timeUnit := time.Millisecond
	remainingRepos := map[string]bool{
		ref1.String(): true,
		ref2.String(): true,
	}

	var mu sync.Mutex
	deleteFunc := func(r reference.Reference) error {
		mu.Lock()
		delete(remainingRepos, r.String())
		mu.Unlock()
		return nil
	}

	fs := inmemory.New()
	pathToStateFile := "/ttl"
	s := New(dcontext.Background(), fs, pathToStateFile)
	s.onBlobExpire = deleteFunc

	err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	s.add(ref1, 300*timeUnit, entryTypeBlob)
	s.add(ref2, 100*timeUnit, entryTypeBlob)

	// Start and stop before all operations complete
	// state will be written to fs
	err = s.Stop()
	if err != nil {
		t.Fatal(err)
	}
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
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}
}

func TestDoubleStart(t *testing.T) {
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	err := s.Start()
	if err != nil {
		t.Fatal("Unable to start scheduler")
	}
	err = s.Start()
	if err == nil {
		t.Fatal("Scheduler started twice without error")
	}
}

func canonicalRef(t *testing.T, s string) reference.Canonical {
	t.Helper()
	ref, err := reference.Parse(s)
	if err != nil {
		t.Fatalf("parse reference %q: %v", s, err)
	}
	c, ok := ref.(reference.Canonical)
	if !ok {
		t.Fatalf("reference %q is not canonical", s)
	}
	return c
}

func TestHasOtherReferencesToDigest(t *testing.T) {
	d1 := digest.Digest("sha256:" + fmt.Sprintf("%064d", 1))
	d2 := digest.Digest("sha256:" + fmt.Sprintf("%064d", 2))
	refA := canonicalRef(t, "repo-a@"+d1.String())
	refB := canonicalRef(t, "repo-b@"+d1.String())
	refC := canonicalRef(t, "repo-c@"+d2.String())

	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	if got := s.HasOtherReferencesToDigest(refA.String(), d1); got {
		t.Fatalf("empty scheduler must return false; got true")
	}

	if err := s.AddBlob(refA, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := s.HasOtherReferencesToDigest(refA.String(), d1); got {
		t.Fatalf("only the excluded entry references d1; want false, got true")
	}

	if err := s.AddBlob(refB, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := s.HasOtherReferencesToDigest(refA.String(), d1); !got {
		t.Fatalf("refB also references d1; want true, got false")
	}

	if got := s.HasOtherReferencesToDigest(refA.String(), d2); got {
		t.Fatalf("d2 unknown; want false, got true")
	}
	if err := s.AddBlob(refC, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := s.HasOtherReferencesToDigest(refA.String(), d2); !got {
		t.Fatalf("refC references d2; want true, got false")
	}
}

func TestAddBlobIfAbsent(t *testing.T) {
	d := digest.Digest("sha256:" + fmt.Sprintf("%064d", 1))
	ref := canonicalRef(t, "repo@"+d.String())

	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	added, err := s.AddBlobIfAbsent(ref, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatalf("first call should have added; got added=false")
	}

	s.Lock()
	first := s.entries[ref.String()]
	s.Unlock()

	added, err = s.AddBlobIfAbsent(ref, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatalf("second call must not have added; got added=true")
	}

	s.Lock()
	second := s.entries[ref.String()]
	s.Unlock()

	if first != second {
		t.Fatalf("AddBlobIfAbsent replaced an existing entry: %p -> %p", first, second)
	}
}

func TestEvictionLock(t *testing.T) {
	// Pick digests whose first encoded byte differs so they land in
	// different stripes of the lock table.
	d := digest.Digest("sha256:11" + fmt.Sprintf("%062d", 1))
	sameStripe := digest.Digest("sha256:11" + fmt.Sprintf("%062d", 2))
	otherStripe := digest.Digest("sha256:22" + fmt.Sprintf("%062d", 1))

	s := New(dcontext.Background(), inmemory.New(), "/ttl-evictlock")

	// Same digest → same mutex instance.
	m1 := s.EvictionLock(d)
	m2 := s.EvictionLock(d)
	if m1 != m2 {
		t.Fatalf("EvictionLock for the same digest must return the same mutex: %p vs %p", m1, m2)
	}
	// Digests in the same stripe share a mutex by design (collisions
	// over-serialise but never break correctness).
	if m1 != s.EvictionLock(sameStripe) {
		t.Fatalf("digests hashing to the same stripe should share a mutex")
	}
	// Different stripe → different mutex (so unrelated digests stay
	// parallel for the common case).
	if m1 == s.EvictionLock(otherStripe) {
		t.Fatalf("digests in different stripes must return distinct mutexes")
	}
	// And it actually locks.
	m1.Lock()
	locked := make(chan struct{})
	go func() {
		m1.Lock()
		close(locked)
		m1.Unlock()
	}()
	select {
	case <-locked:
		t.Fatal("second Lock on the same mutex must block")
	case <-time.After(30 * time.Millisecond):
	}
	m1.Unlock()
	<-locked
}

// TestClaimVacuumSerialisesConcurrentExpiries verifies the protocol
// that protects the shared blob in a concurrent expiry burst: every
// caller atomically drops its own entry and observes the remainder, so
// exactly the last caller in the chain receives the green light to
// vacuum. ClaimVacuum is the primitive that eliminates the
// mutual-skip race that the older HasOtherReferencesToDigest-only path
// required the on-disk walk to mop up.
func TestClaimVacuumSerialisesConcurrentExpiries(t *testing.T) {
	d := digest.Digest("sha256:" + fmt.Sprintf("%064d", 5))
	refs := []reference.Canonical{
		canonicalRef(t, "repo-a@"+d.String()),
		canonicalRef(t, "repo-b@"+d.String()),
		canonicalRef(t, "repo-c@"+d.String()),
	}

	s := New(dcontext.Background(), inmemory.New(), "/ttl-claim")
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	for _, r := range refs {
		if err := s.AddBlob(r, time.Hour); err != nil {
			t.Fatal(err)
		}
	}

	if got := s.ClaimVacuum(refs[0].String(), d); got {
		t.Fatal("first claim must observe siblings; got vacuum=true")
	}
	if got := s.ClaimVacuum(refs[1].String(), d); got {
		t.Fatal("second claim must observe the remaining sibling; got vacuum=true")
	}
	if got := s.ClaimVacuum(refs[2].String(), d); !got {
		t.Fatal("last claim must observe an empty map and proceed to vacuum")
	}

	// Idempotency: a repeat claim on an already-removed entry must not
	// flap or panic, and must still report vacuum=true (no entries).
	if got := s.ClaimVacuum(refs[2].String(), d); !got {
		t.Fatal("repeat claim on a removed entry should still report vacuum=true")
	}
}

func TestAddManifestIfAbsent(t *testing.T) {
	d := digest.Digest("sha256:" + fmt.Sprintf("%064d", 2))
	ref := canonicalRef(t, "repo@"+d.String())

	s := New(dcontext.Background(), inmemory.New(), "/ttl-manifest")
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	added, err := s.AddManifestIfAbsent(ref, time.Hour)
	if err != nil || !added {
		t.Fatalf("first call: added=%v err=%v; want true,nil", added, err)
	}
	added, err = s.AddManifestIfAbsent(ref, time.Hour)
	if err != nil || added {
		t.Fatalf("second call: added=%v err=%v; want false,nil", added, err)
	}
}

func TestStartReconcileRuns(t *testing.T) {
	d := digest.Digest("sha256:" + fmt.Sprintf("%064d", 7))
	ref := canonicalRef(t, "discovered@"+d.String())

	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	var called atomic.Int32
	s.OnReconcile(func(s *TTLExpirationScheduler) error {
		called.Add(1)
		_, err := s.AddBlobIfAbsent(ref, time.Hour)
		return err
	})

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	// Reconcile now runs in a background goroutine; poll Reconciled()
	// instead of asserting synchronously.
	waitForReconciled(t, s, 2*time.Second)

	if got := called.Load(); got != 1 {
		t.Fatalf("reconciler called %d times; want 1", got)
	}

	s.Lock()
	_, present := s.entries[ref.String()]
	s.Unlock()
	if !present {
		t.Fatalf("reconciler discovery was not committed to the entries map")
	}
}

// waitForReconciled polls until s.Reconciled() is true or timeout elapses.
func waitForReconciled(t *testing.T, s *TTLExpirationScheduler, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.Reconciled() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("scheduler did not finish reconcile within %s", timeout)
}

func TestStartReconcileErrorIsNonFatal(t *testing.T) {
	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	sentinel := errors.New("reconcile boom")
	s.OnReconcile(func(*TTLExpirationScheduler) error {
		return sentinel
	})

	if err := s.Start(); err != nil {
		t.Fatalf("scheduler must not fail Start on reconciler error: %v", err)
	}
	defer s.Stop()
}

// TestCallbackCanReenterScheduler asserts that the OnBlobExpire callback
// can call HasOtherReferencesToDigest without deadlocking. Before the
// startTimer lock-split fix this would hang.
func TestCallbackCanReenterScheduler(t *testing.T) {
	d := digest.Digest("sha256:" + fmt.Sprintf("%064d", 9))
	ref := canonicalRef(t, "repo@"+d.String())

	s := New(dcontext.Background(), inmemory.New(), "/ttl")
	done := make(chan bool, 1)
	s.OnBlobExpire(func(r reference.Reference) error {
		c := r.(reference.Canonical)
		done <- s.HasOtherReferencesToDigest(c.String(), c.Digest())
		return nil
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	if err := s.AddBlob(ref, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-done:
		if got {
			t.Fatalf("only one ref scheduled; want false, got true")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("OnBlobExpire callback deadlocked")
	}
}
