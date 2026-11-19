package scheduler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

// Reconciler discovers references that should be tracked by the scheduler but
// are absent from its in-memory state (e.g. orphan on-disk links produced by
// an unclean shutdown or by an older binary). Implementations publish results
// through the scheduler's AddBlobIfAbsent / AddManifestIfAbsent methods.
type Reconciler func(s *TTLExpirationScheduler) error

// onTTLExpiryFunc is called when a repository's TTL expires
type expiryFunc func(reference.Reference) error

const (
	entryTypeBlob = iota
	entryTypeManifest
	indexSaveFrequency = 5 * time.Second

	// evictLockStripes bounds the per-digest eviction lock pool. Digests
	// are hashed (taking the first byte of the encoded hex) into one of
	// these stripes; collisions only over-serialise unrelated digests and
	// never cause a correctness issue. 256 keeps the table tiny while
	// reducing contention to a negligible level for realistic working
	// sets.
	evictLockStripes = 256
)

// schedulerEntry represents an entry in the scheduler
// fields are exported for serialization
type schedulerEntry struct {
	Key       string    `json:"Key"`
	Expiry    time.Time `json:"ExpiryData"`
	EntryType int       `json:"EntryType"`

	timer *time.Timer
}

// New returns a new instance of the scheduler
func New(ctx context.Context, driver driver.StorageDriver, path string) *TTLExpirationScheduler {
	return &TTLExpirationScheduler{
		entries:         make(map[string]*schedulerEntry),
		driver:          driver,
		pathToStateFile: path,
		ctx:             ctx,
		stopped:         true,
		doneChan:        make(chan struct{}),
		saveTimer:       time.NewTicker(indexSaveFrequency),
		evictLocks:      new([evictLockStripes]sync.Mutex),
	}
}

// TTLExpirationScheduler is a scheduler used to perform actions
// when TTLs expire
type TTLExpirationScheduler struct {
	sync.Mutex

	entries map[string]*schedulerEntry

	driver          driver.StorageDriver
	ctx             context.Context
	pathToStateFile string

	stopped bool

	onBlobExpire     expiryFunc
	onManifestExpire expiryFunc
	onReconcile      Reconciler

	// evictLocks serialises OnBlobExpire processing per digest so that N
	// timers firing in the same instant for N repos sharing one digest
	// execute their full delete-link / check-refs / maybe-vacuum cycle one
	// after the other instead of racing. Without serialisation each
	// callback would clear its link, see the other entries (still in the
	// map until the in-flight callback returns) or the other in-flight
	// links on disk, conclude "another ref exists", and skip the vacuum —
	// leaking the blob with no scheduler entry left to recover it.
	// Concurrency across *different* digests stays parallel. The pool is
	// a fixed-size striped lock table (see evictLockStripes) so memory
	// stays bounded regardless of how many distinct digests are seen over
	// the lifetime of the scheduler.
	evictLocks *[evictLockStripes]sync.Mutex

	// reconciled flips to true once the bootstrap reconcile goroutine has
	// finished registering every on-disk link with the scheduler. After
	// that point the scheduler entries are the authoritative reference
	// count for shared blobs and the on-disk link walk in evictBlob can
	// be skipped.
	reconciled atomic.Bool

	indexDirty bool
	saveTimer  *time.Ticker
	doneChan   chan struct{}
}

// OnBlobExpire is called when a scheduled blob's TTL expires
func (ttles *TTLExpirationScheduler) OnBlobExpire(f expiryFunc) {
	ttles.Lock()
	defer ttles.Unlock()

	ttles.onBlobExpire = f
}

// OnManifestExpire is called when a scheduled manifest's TTL expires
func (ttles *TTLExpirationScheduler) OnManifestExpire(f expiryFunc) {
	ttles.Lock()
	defer ttles.Unlock()

	ttles.onManifestExpire = f
}

// OnReconcile registers a one-shot reconciler invoked from Start after the
// persisted state has been loaded. The reconciler runs without holding the
// scheduler mutex and publishes discoveries through AddBlobIfAbsent /
// AddManifestIfAbsent. Must be called before Start.
func (ttles *TTLExpirationScheduler) OnReconcile(r Reconciler) {
	ttles.Lock()
	defer ttles.Unlock()

	ttles.onReconcile = r
}

// AddBlob schedules a blob cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddBlob(blobRef reference.Canonical, ttl time.Duration) error {
	ttles.Lock()
	defer ttles.Unlock()

	if ttles.stopped {
		return fmt.Errorf("scheduler not started")
	}

	ttles.add(blobRef, ttl, entryTypeBlob)
	return nil
}

// AddManifest schedules a manifest cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddManifest(manifestRef reference.Canonical, ttl time.Duration) error {
	ttles.Lock()
	defer ttles.Unlock()

	if ttles.stopped {
		return fmt.Errorf("scheduler not started")
	}

	ttles.add(manifestRef, ttl, entryTypeManifest)
	return nil
}

// AddBlobIfAbsent schedules a blob cleanup only when no entry exists for
// blobRef. Used by bootstrap reconcile to register orphan links found on
// disk without overwriting state already loaded from scheduler-state.json.
// Returns true when a new entry was created.
func (ttles *TTLExpirationScheduler) AddBlobIfAbsent(blobRef reference.Canonical, ttl time.Duration) (bool, error) {
	ttles.Lock()
	defer ttles.Unlock()

	if ttles.stopped {
		return false, fmt.Errorf("scheduler not started")
	}

	if _, present := ttles.entries[blobRef.String()]; present {
		return false, nil
	}
	ttles.add(blobRef, ttl, entryTypeBlob)
	return true, nil
}

// AddManifestIfAbsent schedules a manifest cleanup only when no entry
// exists for manifestRef. See AddBlobIfAbsent.
func (ttles *TTLExpirationScheduler) AddManifestIfAbsent(manifestRef reference.Canonical, ttl time.Duration) (bool, error) {
	ttles.Lock()
	defer ttles.Unlock()

	if ttles.stopped {
		return false, fmt.Errorf("scheduler not started")
	}

	if _, present := ttles.entries[manifestRef.String()]; present {
		return false, nil
	}
	ttles.add(manifestRef, ttl, entryTypeManifest)
	return true, nil
}

// EvictionLock returns a striped mutex used to serialise OnBlobExpire
// processing across all repositories sharing dgst. The proxy callback
// acquires this lock for the full delete-link / ref-count / vacuum
// sequence so that concurrent expiries for the same digest do not race
// each other into a blob leak. Digests are hashed into a fixed-size
// table (evictLockStripes); two digests hashing to the same stripe are
// over-serialised but never deadlock or lose correctness. Memory is
// O(evictLockStripes), independent of how many digests are seen.
func (ttles *TTLExpirationScheduler) EvictionLock(dgst digest.Digest) *sync.Mutex {
	return &ttles.evictLocks[stripeIndex(dgst)]
}

// Reconciled reports whether bootstrap reconcile has finished registering
// every on-disk link with the scheduler. Once true, the scheduler's
// entries map is the authoritative reference count for shared blobs and
// callers may skip any compensating on-disk scans.
func (ttles *TTLExpirationScheduler) Reconciled() bool {
	return ttles.reconciled.Load()
}

// stripeIndex picks a stripe for dgst by taking the first byte of the
// encoded portion as a hex pair. Digest encodings are always hex, so
// this distributes evenly across the 256 stripes; non-conforming inputs
// fall back to stripe 0 (still correct, just slightly more contention).
func stripeIndex(dgst digest.Digest) uint8 {
	enc := dgst.Encoded()
	if len(enc) < 2 {
		return 0
	}
	b, err := hex.DecodeString(enc[:2])
	if err != nil || len(b) == 0 {
		return 0
	}
	return b[0]
}

// HasOtherReferencesToDigest reports whether any scheduled entry other
// than excludeKey targets dgst. Presence in the entries map is the
// authoritative signal: an entry only leaves the map once it has been
// processed (via ClaimVacuum from the eviction callback or via the
// scheduler's timer cleanup), so an entry whose timer has fired but
// whose callback has not yet run still counts as a live reference. This
// is intended as a read-only query; eviction code paths should call
// ClaimVacuum so the remove-self and check-others happen atomically.
func (ttles *TTLExpirationScheduler) HasOtherReferencesToDigest(excludeKey string, dgst digest.Digest) bool {
	ttles.Lock()
	defer ttles.Unlock()
	return ttles.hasOtherRefsLocked(excludeKey, dgst)
}

// ClaimVacuum atomically removes the entry keyed by selfKey and reports
// whether the caller may safely vacuum the shared blob for dgst (true
// means no other scheduled entry references the digest). The eviction
// callback must hold the per-digest EvictionLock when calling this so
// the remove-self / check-siblings pair stays consistent against
// concurrent expiries for the same digest. Atomic remove-then-check is
// what prevents the mutual-skip race during a concurrent expiry burst:
// once an entry has been claimed it cannot be observed by a later
// caller, so callers naturally form a single chain in which only the
// last one sees an empty map and proceeds to vacuum.
func (ttles *TTLExpirationScheduler) ClaimVacuum(selfKey string, dgst digest.Digest) bool {
	ttles.Lock()
	defer ttles.Unlock()
	if _, ok := ttles.entries[selfKey]; ok {
		delete(ttles.entries, selfKey)
		ttles.indexDirty = true
	}
	return !ttles.hasOtherRefsLocked(selfKey, dgst)
}

// hasOtherRefsLocked must be called with ttles.Mutex held. It scans the
// entries map for any reference to dgst other than excludeKey.
// Canonical reference.String() is "<name>@<digest>"; the suffix match
// uses the last '@' because repository names may contain ':' but not
// '@'.
func (ttles *TTLExpirationScheduler) hasOtherRefsLocked(excludeKey string, dgst digest.Digest) bool {
	suffix := dgst.String()
	for key := range ttles.entries {
		if key == excludeKey {
			continue
		}
		idx := strings.LastIndexByte(key, '@')
		if idx < 0 {
			continue
		}
		if key[idx+1:] == suffix {
			return true
		}
	}
	return false
}

// Start starts the scheduler
func (ttles *TTLExpirationScheduler) Start() error {
	ttles.Lock()

	if err := ttles.readState(); err != nil {
		ttles.Unlock()
		return err
	}

	if !ttles.stopped {
		ttles.Unlock()
		return fmt.Errorf("scheduler already started")
	}

	dcontext.GetLogger(ttles.ctx).Infof("Starting cached object TTL expiration scheduler...")
	ttles.stopped = false

	// Start timer for each deserialized entry before releasing the lock so
	// that any expiry that fires concurrently with reconcile cannot race
	// against an unarmed entry.
	for _, entry := range ttles.entries {
		entry.timer = ttles.startTimer(entry, time.Until(entry.Expiry))
	}

	reconciler := ttles.onReconcile
	ttles.Unlock()

	// Reconcile runs in a goroutine so the (potentially slow on object
	// storage) full repositories walk does not block registry startup. It
	// publishes discoveries via AddBlobIfAbsent / AddManifestIfAbsent,
	// which take the lock per-entry. Until the goroutine flips reconciled
	// to true, evictBlob falls back to its on-disk safety walk so
	// orphan links not yet registered cannot be vacuumed prematurely.
	if reconciler == nil {
		ttles.reconciled.Store(true)
	} else {
		go func() {
			if err := reconciler(ttles); err != nil {
				dcontext.GetLogger(ttles.ctx).Warnf("scheduler bootstrap reconcile failed (continuing): %v", err)
			}
			ttles.reconciled.Store(true)
		}()
	}

	// Start a ticker to periodically save the entries index

	go func() {
		for {
			select {
			case <-ttles.saveTimer.C:
				ttles.Lock()
				if !ttles.indexDirty {
					ttles.Unlock()
					continue
				}

				err := ttles.writeState()
				if err != nil {
					dcontext.GetLogger(ttles.ctx).Errorf("Error writing scheduler state: %s", err)
				} else {
					ttles.indexDirty = false
				}
				ttles.Unlock()

			case <-ttles.doneChan:
				return
			}
		}
	}()

	return nil
}

func (ttles *TTLExpirationScheduler) add(r reference.Reference, ttl time.Duration, eType int) {
	entry := &schedulerEntry{
		Key:       r.String(),
		Expiry:    time.Now().Add(ttl),
		EntryType: eType,
	}
	dcontext.GetLogger(ttles.ctx).Infof("Adding new scheduler entry for %s with ttl=%s", entry.Key, time.Until(entry.Expiry))
	if oldEntry, present := ttles.entries[entry.Key]; present && oldEntry.timer != nil {
		oldEntry.timer.Stop()
	}
	ttles.entries[entry.Key] = entry
	entry.timer = ttles.startTimer(entry, ttl)
	ttles.indexDirty = true
}

func (ttles *TTLExpirationScheduler) startTimer(entry *schedulerEntry, ttl time.Duration) *time.Timer {
	return time.AfterFunc(ttl, func() {
		// Resolve the callback under the lock, then release it so the
		// callback can perform I/O and call back into the scheduler
		// (e.g. HasOtherReferencesToDigest) without deadlocking on the
		// non-reentrant sync.Mutex.
		ttles.Lock()
		var f expiryFunc
		switch entry.EntryType {
		case entryTypeBlob:
			f = ttles.onBlobExpire
		case entryTypeManifest:
			f = ttles.onManifestExpire
		default:
			f = func(reference.Reference) error {
				return fmt.Errorf("scheduler entry type")
			}
		}
		ttles.Unlock()

		ref, err := reference.Parse(entry.Key)
		if err == nil {
			if err := f(ref); err != nil {
				dcontext.GetLogger(ttles.ctx).Errorf("Scheduler error returned from OnExpire(%s): %s", entry.Key, err)
			}
		} else {
			dcontext.GetLogger(ttles.ctx).Errorf("Error unpacking reference: %s", err)
		}

		// Re-acquire to remove the entry, but only if the map still holds
		// this exact entry pointer. A concurrent AddBlob may have replaced
		// it with a fresh one (new timer) that must not be evicted.
		ttles.Lock()
		if cur, ok := ttles.entries[entry.Key]; ok && cur == entry {
			delete(ttles.entries, entry.Key)
			ttles.indexDirty = true
		}
		ttles.Unlock()
	})
}

// Stop stops the scheduler.
func (ttles *TTLExpirationScheduler) Stop() error {
	ttles.Lock()
	defer ttles.Unlock()

	err := ttles.writeState()
	if err != nil {
		err = fmt.Errorf("error writing scheduler state: %w", err)
	}

	for _, entry := range ttles.entries {
		entry.timer.Stop()
	}

	close(ttles.doneChan)
	ttles.saveTimer.Stop()
	ttles.stopped = true
	return err
}

func (ttles *TTLExpirationScheduler) writeState() error {
	jsonBytes, err := json.Marshal(ttles.entries)
	if err != nil {
		return err
	}

	err = ttles.driver.PutContent(ttles.ctx, ttles.pathToStateFile, jsonBytes)
	if err != nil {
		return err
	}

	return nil
}

func (ttles *TTLExpirationScheduler) readState() error {
	if _, err := ttles.driver.Stat(ttles.ctx, ttles.pathToStateFile); err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			return nil
		default:
			return err
		}
	}

	bytes, err := ttles.driver.GetContent(ttles.ctx, ttles.pathToStateFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, &ttles.entries)
	if err != nil {
		return err
	}
	return nil
}
