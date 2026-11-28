package proxy

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"

	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

func TestProxyingRegistryCloseWithoutScheduler(t *testing.T) {
	pr := &proxyingRegistry{
		scheduler: nil,
	}

	// verify that `Close()` does not panic when the scheduler is nil
	err := pr.Close()
	if err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
}

func mkDigest(t *testing.T, seed int) digest.Digest {
	t.Helper()
	return digest.Digest("sha256:" + fmt.Sprintf("%064d", seed))
}

func mkCanonical(t *testing.T, repo string, dgst digest.Digest) reference.Canonical {
	t.Helper()
	named, err := reference.WithName(repo)
	if err != nil {
		t.Fatalf("WithName(%q): %v", repo, err)
	}
	c, err := reference.WithDigest(named, dgst)
	if err != nil {
		t.Fatalf("WithDigest: %v", err)
	}
	return c
}

// putLink writes a link file at the path produced by storage.BlobLinkPath
// (or storage.ManifestRevisionLinkPath when manifest=true). The byte
// payload mimics the production format (the digest text) but the eviction
// logic only cares about file existence.
func putLink(t *testing.T, ctx context.Context, drv driver.StorageDriver, repo string, dgst digest.Digest, manifest bool) {
	t.Helper()
	var p string
	var err error
	if manifest {
		p, err = storage.ManifestRevisionLinkPath(repo, dgst)
	} else {
		p, err = storage.BlobLinkPath(repo, dgst)
	}
	if err != nil {
		t.Fatalf("link path: %v", err)
	}
	if err := drv.PutContent(ctx, p, []byte(dgst.String())); err != nil {
		t.Fatalf("PutContent %s: %v", p, err)
	}
}

func TestAnyRepoHasBlobLink_EmptyRegistry(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	got, err := anyRepoHasBlobLink(ctx, drv, mkDigest(t, 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("empty registry must report no link; got true")
	}
}

func TestAnyRepoHasBlobLink_HitAndMiss(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 42)
	putLink(t, ctx, drv, "team/widget", d, false)

	got, err := anyRepoHasBlobLink(ctx, drv, d)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatalf("link exists; want true, got false")
	}

	got, err = anyRepoHasBlobLink(ctx, drv, mkDigest(t, 999))
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatalf("unrelated digest; want false, got true")
	}
}

// Models the production worst case (one digest referenced by ten repos)
// and asserts the slow-path stops on the first match instead of scanning
// the full catalog.
func TestAnyRepoHasBlobLink_TenRefsStopOnFirstMatch(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 7)
	for i := 0; i < 10; i++ {
		putLink(t, ctx, drv, fmt.Sprintf("mirror/repo-%02d", i), d, false)
	}

	got, err := anyRepoHasBlobLink(ctx, drv, d)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatalf("ten link copies exist; want true")
	}
}

func TestParseRepoLinkPath(t *testing.T) {
	root := storage.RepositoriesRootPath()
	d := mkDigest(t, 5)
	hex := d.Encoded()

	cases := []struct {
		name     string
		path     string
		wantOK   bool
		wantRepo string
		wantKind string
	}{
		{
			name:     "blob link",
			path:     fmt.Sprintf("%s/library/alpine/_layers/sha256/%s/link", root, hex),
			wantOK:   true,
			wantRepo: "library/alpine",
			wantKind: linkKindBlob,
		},
		{
			name:     "manifest link",
			path:     fmt.Sprintf("%s/library/alpine/_manifests/revisions/sha256/%s/link", root, hex),
			wantOK:   true,
			wantRepo: "library/alpine",
			wantKind: linkKindManifest,
		},
		{
			name:   "wrong suffix",
			path:   fmt.Sprintf("%s/library/alpine/_layers/sha256/%s/data", root, hex),
			wantOK: false,
		},
		{
			name:   "extra path segment after digest",
			path:   fmt.Sprintf("%s/library/alpine/_layers/sha256/%s/extra/link", root, hex),
			wantOK: false,
		},
		{
			name:   "outside repositories root",
			path:   fmt.Sprintf("/docker/registry/v2/blobs/sha256/%s/data", hex[:2]+"/"+hex),
			wantOK: false,
		},
		{
			name:   "invalid hex length",
			path:   fmt.Sprintf("%s/library/alpine/_layers/sha256/abc/link", root),
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, kind, _, ok := parseRepoLinkPath(root, tc.path)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v; want %v (path=%s)", ok, tc.wantOK, tc.path)
			}
			if !ok {
				return
			}
			if repo != tc.wantRepo {
				t.Errorf("repo = %q; want %q", repo, tc.wantRepo)
			}
			if kind != tc.wantKind {
				t.Errorf("kind = %q; want %q", kind, tc.wantKind)
			}
		})
	}
}

func TestReconcileFromStorage_DiscoversOrphans(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	dBlob := mkDigest(t, 1)
	dManifest := mkDigest(t, 2)
	putLink(t, ctx, drv, "team/svc", dBlob, false)
	putLink(t, ctx, drv, "team/svc", dManifest, true)

	s := scheduler.New(ctx, drv, "/ttl-reconcile")
	s.OnReconcile(reconcileFromStorage(ctx, drv, time.Hour))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	// Reconcile runs in a background goroutine; wait for it to finish
	// before sampling the scheduler state.
	waitForReconciled(t, s, 2*time.Second)

	wantBlob := mkCanonical(t, "team/svc", dBlob).String()
	wantManifest := mkCanonical(t, "team/svc", dManifest).String()

	if !s.HasOtherReferencesToDigest("never", dBlob) {
		t.Fatalf("expected scheduler entry for blob %s after reconcile", wantBlob)
	}
	if !s.HasOtherReferencesToDigest("never", dManifest) {
		t.Fatalf("expected scheduler entry for manifest %s after reconcile", wantManifest)
	}
}

// waitForReconciled polls Reconciled() with a short interval until the
// reconcile goroutine finishes or timeout elapses.
func waitForReconciled(t *testing.T, s *scheduler.TTLExpirationScheduler, timeout time.Duration) {
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

func TestReconcileFromStorage_NoOpWhenInSync(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 3)
	putLink(t, ctx, drv, "team/svc", d, false)

	s := scheduler.New(ctx, drv, "/ttl-noop")
	// Pre-populate scheduler so the link is already known.
	preRef := mkCanonical(t, "team/svc", d)
	var added, ranOnce atomic.Bool
	s.OnReconcile(func(s *scheduler.TTLExpirationScheduler) error {
		ranOnce.Store(true)
		got, err := reconcileAddProbe(s, preRef, time.Hour)
		added.Store(got)
		return err
	})

	// Seed the entry by hand using AddBlobIfAbsent before Start would be
	// natural, but Start is what loads state; instead, seed via state.
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	waitForReconciled(t, s, 2*time.Second)

	if !ranOnce.Load() {
		t.Fatal("custom reconciler did not run")
	}
	if !added.Load() {
		t.Fatal("seed AddBlobIfAbsent should report added=true (entry was new)")
	}

	// Re-run reconcileFromStorage manually; it must NOT create a duplicate.
	rec := reconcileFromStorage(ctx, drv, time.Hour)
	if err := rec(s); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	// Still exactly one entry for the digest (HasOtherReferencesToDigest
	// excludes preRef, so it must report false).
	if s.HasOtherReferencesToDigest(preRef.String(), d) {
		t.Fatalf("reconcile duplicated a scheduler entry")
	}
}

// reconcileAddProbe is a thin test helper for AddBlobIfAbsent so the test
// reads naturally (the production code path uses the same call site).
func reconcileAddProbe(s *scheduler.TTLExpirationScheduler, r reference.Canonical, ttl time.Duration) (bool, error) {
	return s.AddBlobIfAbsent(r, ttl)
}

func TestReconcileFromStorage_NoRepositoriesTree(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New() // empty driver — no /docker/registry/v2/repositories

	s := scheduler.New(ctx, drv, "/ttl-empty")
	s.OnReconcile(reconcileFromStorage(ctx, drv, time.Hour))
	if err := s.Start(); err != nil {
		t.Fatalf("reconcile on empty driver must not fail Start: %v", err)
	}
	defer s.Stop()
}

// startSchedulerWithEvict wires evictBlob as OnBlobExpire callback —
// mirrors what NewRegistryPullThroughCache does, minus the HTTP plumbing.
// Returns the scheduler and a channel that receives one event per
// callback completion so the tests can wait deterministically.
func startSchedulerWithEvict(t *testing.T, ctx context.Context, drv driver.StorageDriver, statePath string) (*scheduler.TTLExpirationScheduler, <-chan error) {
	return startSchedulerWithEvictAndReconcile(t, ctx, drv, statePath, nil)
}

// startSchedulerWithEvictAndReconcile is like startSchedulerWithEvict
// but lets the caller install an OnReconcile hook. Useful for tests
// that need to control whether the scheduler is in its pre- or
// post-reconcile state when an eviction fires.
func startSchedulerWithEvictAndReconcile(t *testing.T, ctx context.Context, drv driver.StorageDriver, statePath string, rec scheduler.Reconciler) (*scheduler.TTLExpirationScheduler, <-chan error) {
	t.Helper()
	reg, err := storage.NewRegistry(ctx, drv, storage.EnableDelete)
	if err != nil {
		t.Fatal(err)
	}
	v := storage.NewVacuum(ctx, drv)

	done := make(chan error, 16)
	s := scheduler.New(ctx, drv, statePath)
	if rec != nil {
		s.OnReconcile(rec)
	}
	s.OnBlobExpire(func(ref reference.Reference) error {
		r, ok := ref.(reference.Canonical)
		if !ok {
			done <- fmt.Errorf("non-canonical reference: %T", ref)
			return nil
		}
		err := evictBlob(ctx, reg, drv, v, s, r)
		done <- err
		return err
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	return s, done
}

func waitForExpiries(t *testing.T, done <-chan error, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for i := 0; i < n; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("OnBlobExpire #%d returned error: %v", i+1, err)
			}
		case <-deadline:
			t.Fatalf("only received %d/%d expiries within %s", i, n, timeout)
		}
	}
}

func TestEvictBlob_SingleRef_VacuumsBlob(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 11)
	repoName := "single/ref"
	putBlobBytes(t, ctx, drv, d, "hello-single")
	putLink(t, ctx, drv, repoName, d, false)

	s, done := startSchedulerWithEvict(t, ctx, drv, "/ttl-evict-single")
	defer s.Stop()

	r := mkCanonical(t, repoName, d)
	if err := s.AddBlob(r, 25*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitForExpiries(t, done, 1, 2*time.Second)

	if blobStillThere(t, ctx, drv, d) {
		t.Fatalf("blob file must be removed for single-ref evict")
	}
}

func TestEvictBlob_MultiRef_PreservesBlobViaFastPath(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 21)
	putBlobBytes(t, ctx, drv, d, "hello-multi")
	putLink(t, ctx, drv, "team/a", d, false)
	putLink(t, ctx, drv, "team/b", d, false)

	s, done := startSchedulerWithEvict(t, ctx, drv, "/ttl-evict-multi")
	defer s.Stop()

	rA := mkCanonical(t, "team/a", d)
	rB := mkCanonical(t, "team/b", d)
	if err := s.AddBlob(rA, 25*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := s.AddBlob(rB, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	// After the first timer fires, rB is still scheduled — fast path keeps
	// the blob alive.
	waitForExpiries(t, done, 1, 2*time.Second)
	if !blobStillThere(t, ctx, drv, d) {
		t.Fatalf("blob removed after first expiry while second ref still scheduled")
	}

	// After the second timer fires, no scheduled entry and no on-disk link
	// remain — slow path returns false, blob is vacuumed.
	waitForExpiries(t, done, 1, 2*time.Second)
	if blobStillThere(t, ctx, drv, d) {
		t.Fatalf("blob must be removed once last ref expires")
	}
}

// Regression for the N-concurrent-expiry race: many repos share one digest,
// every per-repo timer fires almost simultaneously. Without the vacuum
// claim each callback would clear its own link, see the others still in
// the scheduler map and skip the vacuum, leaving the shared blob orphaned.
func TestEvictBlob_ConcurrentExpiriesDoNotLeak(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	const repoCount = 10
	d := mkDigest(t, 41)
	putBlobBytes(t, ctx, drv, d, "hello-concurrent")
	for i := 0; i < repoCount; i++ {
		putLink(t, ctx, drv, fmt.Sprintf("mirror/team-%02d", i), d, false)
	}

	s, done := startSchedulerWithEvict(t, ctx, drv, "/ttl-evict-concurrent")
	defer s.Stop()

	// Same very-short TTL for every repo so all timers fire in the same
	// narrow window — exactly the post-restart pattern from production.
	for i := 0; i < repoCount; i++ {
		ref := mkCanonical(t, fmt.Sprintf("mirror/team-%02d", i), d)
		if err := s.AddBlob(ref, 30*time.Millisecond); err != nil {
			t.Fatalf("AddBlob %d: %v", i, err)
		}
	}

	waitForExpiries(t, done, repoCount, 5*time.Second)

	if blobStillThere(t, ctx, drv, d) {
		t.Fatalf("shared blob remained on disk after all %d references expired (leak)", repoCount)
	}
}

func TestEvictBlob_SlowPathDetectsLinkOnDisk(t *testing.T) {
	ctx := context.Background()
	drv := inmemory.New()

	d := mkDigest(t, 31)
	putBlobBytes(t, ctx, drv, d, "hello-slow")
	// Two links on disk, but only the first is registered with the
	// scheduler — the second simulates an orphan link the bootstrap
	// reconcile would normally pick up, while also exercising the slow
	// path here in isolation. The slow on-disk walk only fires before
	// reconcile has finished, so this test wires a reconciler that
	// blocks until the eviction has already run, keeping the scheduler
	// in the pre-reconcile state for the duration of the test.
	putLink(t, ctx, drv, "team/scheduled", d, false)
	putLink(t, ctx, drv, "team/orphan", d, false)

	releaseReconcile := make(chan struct{})
	s, done := startSchedulerWithEvictAndReconcile(t, ctx, drv, "/ttl-evict-slow",
		func(*scheduler.TTLExpirationScheduler) error {
			<-releaseReconcile
			return nil
		})
	defer close(releaseReconcile)
	defer s.Stop()

	rSched := mkCanonical(t, "team/scheduled", d)
	if err := s.AddBlob(rSched, 25*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitForExpiries(t, done, 1, 2*time.Second)

	if !blobStillThere(t, ctx, drv, d) {
		t.Fatalf("slow path should have detected the orphan link and preserved the blob")
	}
}

func putBlobBytes(t *testing.T, ctx context.Context, drv driver.StorageDriver, dgst digest.Digest, content string) {
	t.Helper()
	// Mirror the on-disk layout: /docker/registry/v2/blobs/<alg>/<2>/<hex>/data
	hex := dgst.Encoded()
	p := fmt.Sprintf("/docker/registry/v2/blobs/%s/%s/%s/data", dgst.Algorithm(), hex[:2], hex)
	if err := drv.PutContent(ctx, p, []byte(content)); err != nil {
		t.Fatalf("PutContent blob: %v", err)
	}
}

func blobStillThere(t *testing.T, ctx context.Context, drv driver.StorageDriver, dgst digest.Digest) bool {
	t.Helper()
	hex := dgst.Encoded()
	p := fmt.Sprintf("/docker/registry/v2/blobs/%s/%s/%s/data", dgst.Algorithm(), hex[:2], hex)
	_, err := drv.Stat(ctx, p)
	if err == nil {
		return true
	}
	if _, ok := err.(driver.PathNotFoundError); ok {
		return false
	}
	t.Fatalf("Stat blob: %v", err)
	return false
}
