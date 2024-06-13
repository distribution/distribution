package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

// onTTLExpiryFunc is called when a repository's TTL expires
type expiryFunc func(reference.Reference) error

const (
	entryTypeBlob = iota
	entryTypeManifest
	indexSaveFrequency      = 5 * time.Second
	garbageCollectFrequency = 1 * time.Minute
)

// schedulerEntry represents an entry in the scheduler
// fields are exported for serialization
type schedulerEntry struct {
	Key       string    `json:"Key"`
	Expiry    time.Time `json:"ExpiryData"`
	EntryType int       `json:"EntryType"`

	timer *time.Timer
}

func (se schedulerEntry) String() string {
	return fmt.Sprintf("Expiry: %s, EntryType: %d", se.Expiry, se.EntryType)
}

// New returns a new instance of the scheduler
func New(ctx context.Context, driver driver.StorageDriver, path string, ttl *time.Duration, registry distribution.Namespace, opts storage.GCOpts) *TTLExpirationScheduler {
	return &TTLExpirationScheduler{
		entries:             make(map[string]*schedulerEntry),
		driver:              driver,
		pathToStateFile:     path,
		ttl:                 ttl,
		registry:            registry,
		opts:                opts,
		ctx:                 ctx,
		stopped:             true,
		doneChan:            make(chan struct{}),
		saveTimer:           time.NewTicker(indexSaveFrequency),
		garbageCollectTimer: time.NewTicker(garbageCollectFrequency),
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
	ttl             *time.Duration
	registry        distribution.Namespace
	opts            storage.GCOpts

	stopped bool

	onBlobExpire     expiryFunc
	onManifestExpire expiryFunc

	indexDirty          bool
	saveTimer           *time.Ticker
	garbageCollectTimer *time.Ticker
	doneChan            chan struct{}
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

// Start starts the scheduler
func (ttles *TTLExpirationScheduler) Start() error {
	ttles.Lock()
	defer ttles.Unlock()

	err := ttles.readState()
	if err != nil {
		return err
	}

	if !ttles.stopped {
		return fmt.Errorf("scheduler already started")
	}

	dcontext.GetLogger(ttles.ctx).Infof("Starting cached object TTL (cameron changed) expiration scheduler...")
	ttles.stopped = false

	// Start timer for each deserialized entry
	for _, entry := range ttles.entries {
		entry.timer = ttles.startTimer(entry, time.Until(entry.Expiry))
	}

	err = ttles.BackfillManifests()
	if err != nil {
		return fmt.Errorf("failed to backfill manifests: %w", err)
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
				dcontext.GetLogger(ttles.ctx).Debugf("Current state: \n %+v", ttles.entries)

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

	// You could paraallize this, but you'd want to make sure work was not overlapping
	go ttles.GarbageCollect()

	return nil
}

func (ttles *TTLExpirationScheduler) GarbageCollect() {
	for {
		select {
		case <-ttles.garbageCollectTimer.C:

			storage.MarkAndSweep(ttles.ctx, ttles.driver, ttles.registry, ttles.opts)

		case <-ttles.doneChan:
			return
		}
	}
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
		ttles.Lock()
		defer ttles.Unlock()

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

		ref, err := reference.Parse(entry.Key)
		if err == nil {
			if err := f(ref); err != nil {
				dcontext.GetLogger(ttles.ctx).Errorf("Scheduler error returned from OnExpire(%s): %s", entry.Key, err)
			}
		} else {
			dcontext.GetLogger(ttles.ctx).Errorf("Error unpacking reference: %s", err)
		}

		delete(ttles.entries, entry.Key)
		ttles.indexDirty = true
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
	dcontext.GetLogger(ttles.ctx).Infof("Start state: \n %+v", ttles.entries)

	return nil
}

func (ttles *TTLExpirationScheduler) BackfillManifests() error {
	repositoryEnumerator, ok := ttles.registry.(distribution.RepositoryEnumerator)
	if !ok {
		return fmt.Errorf("unable to convert Namespace to RepositoryEnumerator")
	}
	emit("backfilling manifests")

	// mark
	err := repositoryEnumerator.Enumerate(ttles.ctx, func(repoName string) error {
		emit("backfill for " + repoName)

		var err error
		named, err := reference.WithName(repoName)
		if err != nil {
			return fmt.Errorf("failed to parse repo name %s: %v", repoName, err)
		}
		repository, err := ttles.registry.Repository(ttles.ctx, named)
		if err != nil {
			return fmt.Errorf("failed to construct repository: %v", err)
		}

		manifestService, err := repository.Manifests(ttles.ctx)
		if err != nil {
			return fmt.Errorf("failed to construct manifest service: %v", err)
		}

		manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
		if !ok {
			return fmt.Errorf("unable to convert ManifestService into ManifestEnumerator")
		}

		err = manifestEnumerator.Enumerate(ttles.ctx, func(dgst digest.Digest) error {
			// Mark the manifest's blob
			emit("backfill for %s: adding ttl manifest %s ", repoName, dgst)

			// Skip if TTL exists for manifest
			key := dgst.String()
			if _, ok := ttles.entries[key]; !ok {
				ttles.entries[key] = &schedulerEntry{
					Key: key,

					// TODO file created at is probably better
					Expiry:    time.Now().Add(*ttles.ttl),
					EntryType: entryTypeManifest,
				}
			}

			return nil
		})

		// In certain situations such as unfinished uploads, deleting all
		// tags in S3 or removing the _manifests folder manually, this
		// error may be of type PathNotFound.
		//
		// In these cases we can continue marking other manifests safely.
		if _, ok := err.(driver.PathNotFoundError); ok {
			return nil
		}

		return err
	})

	return err
}

func emit(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}
