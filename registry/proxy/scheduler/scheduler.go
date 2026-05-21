package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	// Concurrency across *different* digests stays parallel.
	evictLocks sync.Map // digest.Digest -> *sync.Mutex

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
// Returns added=true when a new entry was created.
func (ttles *TTLExpirationScheduler) AddBlobIfAbsent(blobRef reference.Canonical, ttl time.Duration) (added bool, err error) {
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
func (ttles *TTLExpirationScheduler) AddManifestIfAbsent(manifestRef reference.Canonical, ttl time.Duration) (added bool, err error) {
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

// EvictionLock returns a per-digest mutex used to serialise OnBlobExpire
// processing across all repositories sharing dgst. The proxy callback
// acquires this lock for the full delete-link / ref-count / vacuum
// sequence so that concurrent expiries for the same digest do not race
// each other into a blob leak. The lock is allocated on first use and
// retained for the lifetime of the scheduler — one entry per unique
// digest the scheduler has ever vacuumed, which is bounded by the
// working set and small in practice.
func (ttles *TTLExpirationScheduler) EvictionLock(dgst digest.Digest) *sync.Mutex {
	if m, ok := ttles.evictLocks.Load(dgst); ok {
		return m.(*sync.Mutex)
	}
	actual, _ := ttles.evictLocks.LoadOrStore(dgst, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// HasOtherReferencesToDigest reports whether any scheduled entry other than
// excludeKey targets dgst AND is still live (its Expiry is in the future).
// Entries whose timer has already fired but whose callback has not yet
// returned are excluded — they are on the path to vacuum themselves and
// must not be treated as a live reference, or the fast path turns into a
// mutual-skip deadlock during a concurrent expiry burst.
func (ttles *TTLExpirationScheduler) HasOtherReferencesToDigest(excludeKey string, dgst digest.Digest) bool {
	now := time.Now()
	ttles.Lock()
	defer ttles.Unlock()

	suffix := dgst.String()
	for key, entry := range ttles.entries {
		if key == excludeKey {
			continue
		}
		if !entry.Expiry.After(now) {
			continue
		}
		// Canonical reference.String() is "<name>@<digest>"; split on the
		// last '@' because repository names may contain ':' but not '@'.
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

	// Reconcile runs after the mutex is released so its I/O does not widen
	// the critical section. It publishes via AddBlobIfAbsent /
	// AddManifestIfAbsent, which take the lock per-entry.
	if reconciler != nil {
		if err := reconciler(ttles); err != nil {
			dcontext.GetLogger(ttles.ctx).Warnf("scheduler bootstrap reconcile failed (continuing): %v", err)
		}
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
