package scheduler

// todo: move to own package and make constructor
import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/filesystem"
)

// onTTLExpiryFunc is called when a repositories' TTL expires
type expiryFunc func(string) error

const (
	entryTypeBlob = iota
	entryTypeManifest
	entryTypeSentinel
)

// schedulerEntry represents an entry in the scheduler
// fields are exported for serialization
type schedulerEntry struct {
	Key        string
	ExpiryDate time.Time
	EntryType  int
}

var theScheduler *ttlExpirationScheduler

func init() {
	// todo(richardscother): Find a way to make the driver and path configurable
	theScheduler = new(context.Background(), filesystem.New("/tmp"), "/ttl.json")
}

// ttlExpirationScheduler is a scheduler used to perform actions
// when TTLs expire
type ttlExpirationScheduler struct {
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
func new(ctx context.Context, driver driver.StorageDriver, path string) *ttlExpirationScheduler {
	return &ttlExpirationScheduler{
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
func OnBlobExpire(f expiryFunc) {
	theScheduler.onBlobExpire = f
}

// OnManifestExpire is called when a scheduled manifest's TTL expires
func OnManifestExpire(f expiryFunc) {
	theScheduler.onManifestExpire = f
}

// AddBlob schedules a blob cleanup after ttl expires
func AddBlob(dgst string, ttl time.Duration) error {
	if theScheduler.stopped {
		return fmt.Errorf("scheduler not started")
	}
	theScheduler.add(dgst, ttl, entryTypeBlob)
	return nil
}

// AddManifest schedules a manifest cleanup after ttl expires
func AddManifest(repoName string, ttl time.Duration) error {
	if theScheduler.stopped {
		return fmt.Errorf("scheduler not started")
	}

	theScheduler.add(repoName, ttl, entryTypeManifest)
	return nil
}

// Start starts the scheduler
func Start() error {
	return theScheduler.start()
}

// Add schedules new TTL expiries
func (ttles *ttlExpirationScheduler) add(key string, ttl time.Duration, eType int) {
	entry := schedulerEntry{
		Key:        key,
		ExpiryDate: time.Now().Add(ttl),
		EntryType:  eType,
	}
	ttles.addChan <- entry
}

func (ttles *ttlExpirationScheduler) stop() {
	ttles.stopChan <- true
}

func (ttles *ttlExpirationScheduler) start() error {
	err := ttles.readState()
	if err != nil {
		return err
	}

	if !ttles.stopped {
		return fmt.Errorf("Scheduler already started")
	}

	context.GetLogger(ttles.ctx).Infof("Starting cached object TTL expiration scheduler...")
	ttles.stopped = false
	go ttles.mainloop()

	return nil
}

// mainloop uses a select statement to listen for events.  Most of its time
// is spent in waiting on a TTL to expire but can be interrupted when TTLs
// are added.
func (ttles *ttlExpirationScheduler) mainloop() {
	for {
		if ttles.stopped {
			return
		}

		nextEntry, ttl := nextExpiringEntry(ttles.entries)
		if len(ttles.entries) == 0 {
			context.GetLogger(ttles.ctx).Infof("scheduler mainloop(): Nothing to do, sleeping for %s", ttl)
		} else {
			context.GetLogger(ttles.ctx).Infof("scheduler mainloop(): Sleeping for %s until cleanup of %s", ttl, nextEntry.Key)
		}

		select {
		case <-time.After(ttl):
			var f expiryFunc

			switch nextEntry.EntryType {
			case entryTypeBlob:
				f = ttles.onBlobExpire
			case entryTypeManifest:
				f = ttles.onManifestExpire
			case entryTypeSentinel:
				f = func(repoName string) error {
					// No implementation
					return nil
				}
			default:
				f = func(repoName string) error {
					return fmt.Errorf("Unknown scheduler entry type: %d", nextEntry.EntryType)
				}
			}

			if err := f(nextEntry.Key); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Schedluer error returned from OnExpire(%s): %s", nextEntry.Key, err)
				break
			}

			delete(ttles.entries, nextEntry.Key)

			if err := ttles.writeState(); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error writing scheduler state: %s", err)
			}
		case entry := <-ttles.addChan:
			context.GetLogger(ttles.ctx).Infof("Adding new scheduler entry for %s with ttl=%s", entry.Key, entry.ExpiryDate.Sub(time.Now()))
			ttles.entries[entry.Key] = entry
			if err := ttles.writeState(); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error writing scheduler state: %s", err)
			}
			break

		case <-ttles.stopChan:
			if err := ttles.writeState(); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error writing scheduler state: %s", err)
			}
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

func (ttles *ttlExpirationScheduler) writeState() error {
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

func (ttles *ttlExpirationScheduler) readState() error {
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
