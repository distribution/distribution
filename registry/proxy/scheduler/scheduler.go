package scheduler

// todo: move to own package and make constructor
import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
)

// onTTLExpiryFunc is called when a repositories' TTL expires
type expiryFunc func(string) error

type Scheduler interface {
	Start() error
	Stop()

	AddBlob(string, time.Duration)
	AddManifest(string, time.Duration)

	OnBlobExpire(expiryFunc)
	OnManifestExpire(expiryFunc)
}

const (
	entryTypeBlob = iota
	entryTypeManifest
	entryTypeSentinel
)

type schedulerEntry struct {
	Key        string
	ExpiryDate time.Time
	EntryType  int
}

func (s schedulerEntry) String() string {
	return fmt.Sprintf("schedulerEntry[%s] -> %s", s.Key, s.ExpiryDate.Sub(time.Now()))
}

// TTLExpirationScheduler is a scheduler used to perform actions
// when TTLs expire
type TTLExpirationScheduler struct {
	entries  map[string]schedulerEntry
	addChan  chan schedulerEntry
	stopChan chan bool

	driver          driver.StorageDriver
	ctx             context.Context
	pathToStateFile string

	stopped bool

	onBlobExpire     expiryFunc
	onManifestExpire expiryFunc
}

// addChan allows more TTLs to be pushed to the scheduler
type addChan chan schedulerEntry

// stopChan allows the scheduler to be stopped - used for testing.
type stopChan chan bool

// New returns a new TTL expiration scheduler
func New(ctx context.Context, driver driver.StorageDriver, path string) Scheduler {
	return &TTLExpirationScheduler{
		entries:         make(map[string]schedulerEntry),
		addChan:         make(chan schedulerEntry),
		stopChan:        make(chan bool),
		driver:          driver,
		pathToStateFile: path,
		ctx:             ctx,
		stopped:         true,
	}
}

// OnBlobExpire is called when a scheduled blob's TTL expires
func (ttles *TTLExpirationScheduler) OnBlobExpire(f expiryFunc) {
	ttles.onBlobExpire = f
}

// OnManifestExpire is called when a scheduled manifest's TTL expires
func (ttles *TTLExpirationScheduler) OnManifestExpire(f expiryFunc) {
	ttles.onManifestExpire = f
}

// AddBlob schedules a blob cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddBlob(dgst string, ttl time.Duration) {
	ttles.add(dgst, ttl, entryTypeBlob)
}

// AddManifest schedules a manifest cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddManifest(repoName string, ttl time.Duration) {
	ttles.add(repoName, ttl, entryTypeManifest)
}

// Add schedules new TTL expiries
func (ttles *TTLExpirationScheduler) add(key string, ttl time.Duration, eType int) {
	entry := schedulerEntry{
		Key:        key,
		ExpiryDate: time.Now().Add(ttl),
		EntryType:  eType,
	}
	ttles.addChan <- entry
}

// Stop stops the scheduler - used for testing
func (ttles *TTLExpirationScheduler) Stop() {
	ttles.stopChan <- true
}

// Start calls the given onTTlExpiryFunc when the repositories' TTL expires
func (ttles *TTLExpirationScheduler) Start() error {
	err := ttles.readState()
	if err != nil {
		return err
	}

	if !ttles.stopped {
		return fmt.Errorf("Scheduler already started")
	}

	context.GetLogger(ttles.ctx).Infof("Starting TTL expiration scheduler...")
	ttles.stopped = false
	go ttles.mainloop()

	return nil
}

// mainloop uses a select statement to listen for events.  Most of its time
// is spent in waiting on a TTL to expire but can be interrupted when TTLs
// are added.
func (ttles *TTLExpirationScheduler) mainloop() {
	for {
		if ttles.stopped {
			return
		}

		nextEntry, ttl := nextExpiringEntry(ttles.entries)
		context.GetLogger(ttles.ctx).Infof("mainloop() : Sleeping for %s until cleanup of %s", ttl, nextEntry.Key)

		select {
		case <-time.After(ttl):

			var f expiryFunc

			switch nextEntry.EntryType {
			case entryTypeBlob:
				f = ttles.onBlobExpire
			case entryTypeManifest:
				f = ttles.onManifestExpire
			default:
				f = func(repoName string) error {
					return nil
				}
			}

			if f == nil {
				context.GetLogger(ttles.ctx).Errorf("Nil func: %#v\n", ttles.onBlobExpire)
				delete(ttles.entries, nextEntry.Key)
				break
			}

			if err := f(nextEntry.Key); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error returned from OnExpire(%s): %s", nextEntry.Key, err)
				break
			}

			delete(ttles.entries, nextEntry.Key)

			if err := ttles.writeState(); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error writing TTL state: %s", err)
			}
		case entry := <-ttles.addChan:
			context.GetLogger(ttles.ctx).Errorf("Adding new entry for %s with ttl=%s", entry.Key, entry.ExpiryDate.Sub(time.Now()))
			ttles.entries[entry.Key] = entry
			ttles.writeState()
			break

		case <-ttles.stopChan:
			ttles.writeState()
			ttles.stopped = true
		}
	}
}

func nextExpiringEntry(entries map[string]schedulerEntry) (schedulerEntry, time.Duration) {
	// this is a sentinel entry that will be invoked
	// if there are no other entries in the scheduler
	nextEntry := schedulerEntry{
		Key:        "sentinel-entry",
		ExpiryDate: time.Now().Add(24 * time.Hour),
		EntryType:  entryTypeSentinel,
	}

	for _, entry := range entries {
		if entry.ExpiryDate.Before(nextEntry.ExpiryDate) {
			nextEntry = entry
		}
	}

	// Dates may be from the past if the scheduler has
	// been restarted, set their ttl to 0
	if nextEntry.ExpiryDate.Before(time.Now()) {
		nextEntry.ExpiryDate = time.Now()
		return nextEntry, 0
	}

	return nextEntry, nextEntry.ExpiryDate.Sub(time.Now())
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
