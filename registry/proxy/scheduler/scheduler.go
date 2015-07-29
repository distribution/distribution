package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
)

// onTTLExpiryFunc is called when a repositories' TTL expires
type expiryFunc func(string) error

const (
	entryTypeBlob = iota
	entryTypeManifest
)

// schedulerEntry represents an entry in the scheduler
// fields are exported for serialization
type schedulerEntry struct {
	Key       string    `json:"Key"`
	Expiry    time.Time `json:"ExpiryData"`
	EntryType int       `json:"EntryType"`
}

// New returns a new instance of the scheduler
func New(ctx context.Context, driver driver.StorageDriver, path string) *TTLExpirationScheduler {
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

// OnBlobExpire is called when a scheduled blob's TTL expires
func (ttles *TTLExpirationScheduler) OnBlobExpire(f expiryFunc) {
	ttles.onBlobExpire = f
}

// OnManifestExpire is called when a scheduled manifest's TTL expires
func (ttles *TTLExpirationScheduler) OnManifestExpire(f expiryFunc) {
	ttles.onManifestExpire = f
}

// AddBlob schedules a blob cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddBlob(dgst string, ttl time.Duration) error {
	if ttles.stopped {
		return fmt.Errorf("scheduler not started")
	}
	ttles.add(dgst, ttl, entryTypeBlob)
	return nil
}

// AddManifest schedules a manifest cleanup after ttl expires
func (ttles *TTLExpirationScheduler) AddManifest(repoName string, ttl time.Duration) error {
	if ttles.stopped {
		return fmt.Errorf("scheduler not started")
	}

	ttles.add(repoName, ttl, entryTypeManifest)
	return nil
}

// Start starts the scheduler
func (ttles *TTLExpirationScheduler) Start() error {
	return ttles.start()
}

func (ttles *TTLExpirationScheduler) add(key string, ttl time.Duration, eType int) {
	entry := schedulerEntry{
		Key:       key,
		Expiry:    time.Now().Add(ttl),
		EntryType: eType,
	}
	ttles.addChan <- entry
}

func (ttles *TTLExpirationScheduler) stop() {
	ttles.stopChan <- true
}

func (ttles *TTLExpirationScheduler) start() error {
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
func (ttles *TTLExpirationScheduler) mainloop() {
	for {
		if ttles.stopped {
			return
		}

		nextEntry, ttl := nextExpiringEntry(ttles.entries)
		if len(ttles.entries) == 0 {
			context.GetLogger(ttles.ctx).Infof("scheduler mainloop(): Nothing to do, sleeping...")
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
			default:
				f = func(repoName string) error {
					return fmt.Errorf("Unexpected scheduler entry type")
				}
			}

			if err := f(nextEntry.Key); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Scheduler error returned from OnExpire(%s): %s", nextEntry.Key, err)
			}

			delete(ttles.entries, nextEntry.Key)
			if err := ttles.writeState(); err != nil {
				context.GetLogger(ttles.ctx).Errorf("Error writing scheduler state: %s", err)
			}
		case entry := <-ttles.addChan:
			context.GetLogger(ttles.ctx).Infof("Adding new scheduler entry for %s with ttl=%s", entry.Key, entry.Expiry.Sub(time.Now()))
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

func nextExpiringEntry(entries map[string]schedulerEntry) (*schedulerEntry, time.Duration) {
	if len(entries) == 0 {
		return nil, 24 * time.Hour
	}

	// todo:(richardscothern) this is a primitive o(n) algorithm
	// but n will never be *that* big and it's all in memory.  Investigate
	// time.AfterFunc for heap based expiries

	first := true
	var nextEntry schedulerEntry
	for _, entry := range entries {
		if first {
			nextEntry = entry
			first = false
			continue
		}
		if entry.Expiry.Before(nextEntry.Expiry) {
			nextEntry = entry
		}
	}

	// Dates may be from the past if the scheduler has
	// been restarted, set their ttl to 0
	if nextEntry.Expiry.Before(time.Now()) {
		nextEntry.Expiry = time.Now()
		return &nextEntry, 0
	}

	return &nextEntry, nextEntry.Expiry.Sub(time.Now())
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
