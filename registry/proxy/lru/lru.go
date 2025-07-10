package lru

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/proxy"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/sirupsen/logrus"
)

const (
	entryTypeBlob = iota
	entryTypeManifest
	entryTypeMetadata
	indexSaveFrequency = 5 * time.Second
	entryNameHead      = "HEAD"
	entryNameTail      = "TAIL"
)

var _ proxy.EvictionController = &LRUEvictionController{}

// init registers the LRU eviction backend.
func init() {
	if err := proxy.Register("lru", proxy.InitFunc(new)); err != nil {
		logrus.Errorf("failed to register LRU eviction controller: %v", err)
	}
}

// lruEntry represents an entry in the LRU cache
// it differs from schedulerEntry in that the CanonicalReference is split between `Key` and `References`.
// `References` contain the Named portion of the CanonicalReference(s) and is necessary to delete every link in the LinkedBlobstore.
// `Key` contains the Digest, which used to deduplicate images that share a base layer (useful for `touch`).
// since a `Digest` is of form (digest-algorithm ":" digest-hex), they will not collided with the special entries for head and tail.
// `Prev` and `Next` are `Key`s for the underlying doubly linked list structure used in insertion and eviction.
type lruEntry struct {
	Key        string   `json:"Key"`
	EntryType  int      `json:"EntryType"`
	References []string `json:"References"`
	Prev       string   `json:"Prev"`
	Next       string   `json:"Next"`
}

// EvictionQueue is the underlying data structure for the LRU: a hash map + doubly linked list. Consider the following example.
//
// HEAD <-> Entry1 <-> Entry2 <-> TAIL
//
// The HEAD and TAIL entries are special entries to maintain the structure and should always exist.
// The entry after HEAD should be the least recently used and the the entry before TAIL the most recently used.
// If a new entry `Entry3` is `add`ed, then, it will be between `Entry2` and TAIL.
// If `Entry1` is used again, it will be `touch`ed and moved to the end of the eviction queue (i.e. right before TAIL).
//
// It might benefit to factor out the linked list operations to this type in the future for better testing and isolation.
type EvictionQueue map[string]*lruEntry

// new returns a new instance of the LRU eviction controller
func new(ctx context.Context, driver driver.StorageDriver, path string, options map[string]interface{}) (proxy.EvictionController, error) {
	limitOpt, present := options["limit"]
	if !present {
		return nil, fmt.Errorf(`"limit" must be set for LRU eviction policy`)
	}
	limitStr, ok := limitOpt.(string)
	if !ok {
		return nil, fmt.Errorf(`"limit" must be a valid positive size capacity value, given %v`, limitOpt)
	}
	limit, err := datasize.ParseString(limitStr)
	if err != nil || limit <= 0 {
		return nil, fmt.Errorf(`"limit" must be a valid positive size capacity value, given %v, with error: %v`, limit, err)
	}

	// special head and tail entries are exported to simplify using the doubly linked list
	headEntry := lruEntry{
		Key:       entryNameHead,
		EntryType: entryTypeMetadata,
		Prev:      entryNameHead,
		Next:      entryNameTail,
	}
	tailEntry := lruEntry{
		Key:       entryNameTail,
		EntryType: entryTypeMetadata,
		Prev:      entryNameHead,
		Next:      entryNameTail,
	}

	lruec := LRUEvictionController{
		entries: EvictionQueue{
			entryNameHead: &headEntry,
			entryNameTail: &tailEntry,
		},
		head:            &headEntry,
		tail:            &tailEntry,
		driver:          driver,
		pathToStateFile: path,
		limit:           limit.Bytes(),
		ctx:             ctx,
		stopped:         true,
		doneChan:        make(chan struct{}),
		saveTimer:       time.NewTicker(indexSaveFrequency),
	}

	return &lruec, nil
}

// LRUEvictionController is an eviction controller used to perform actions
// when the specified space limit is exceeded
type LRUEvictionController struct {
	sync.Mutex

	entries EvictionQueue
	head    *lruEntry
	tail    *lruEntry

	driver          driver.StorageDriver
	ctx             context.Context
	pathToStateFile string
	limit           uint64

	stopped bool

	onBlobExpire     proxy.OnEvictFunc
	onManifestExpire proxy.OnEvictFunc

	indexDirty bool
	saveTimer  *time.Ticker
	doneChan   chan struct{}
}

// OnBlobEvict registers the callback function for when a cached blob is evicted
func (lruec *LRUEvictionController) OnBlobEvict(f proxy.OnEvictFunc) {
	lruec.Lock()
	defer lruec.Unlock()

	lruec.onBlobExpire = f
}

// OnManifestEvict registers the callback function for when a cached manifest is evicted
func (lruec *LRUEvictionController) OnManifestEvict(f proxy.OnEvictFunc) {
	lruec.Lock()
	defer lruec.Unlock()

	lruec.onManifestExpire = f
}

// AddBlob adds a blob to the LRU
func (lruec *LRUEvictionController) AddBlob(blobRef reference.Canonical) error {
	lruec.Lock()
	defer lruec.Unlock()

	if lruec.stopped {
		return fmt.Errorf("LRU eviction controller not started")
	}

	if err := lruec.add(blobRef, entryTypeBlob); err != nil {
		return err
	}
	return nil
}

// AddManifest adds a manifest to the LRU
func (lruec *LRUEvictionController) AddManifest(manifestRef reference.Canonical) error {
	lruec.Lock()
	defer lruec.Unlock()

	if lruec.stopped {
		return fmt.Errorf("LRU eviction controller not started")
	}

	if err := lruec.add(manifestRef, entryTypeManifest); err != nil {
		return err
	}
	return nil
}

// AddBlob touches the LRU entry from a cache hit
func (lruec *LRUEvictionController) TouchBlob(blobRef reference.Canonical) error {
	lruec.Lock()
	defer lruec.Unlock()

	if lruec.stopped {
		return fmt.Errorf("LRU eviction controller not started")
	}

	if err := lruec.touch(blobRef, entryTypeBlob); err != nil {
		return err
	}
	return nil
}

// AddManifest touches the LRU entry from a cache hit
func (lruec *LRUEvictionController) TouchManifest(manifestRef reference.Canonical) error {
	lruec.Lock()
	defer lruec.Unlock()

	if lruec.stopped {
		return fmt.Errorf("LRU eviction controller not started")
	}

	if err := lruec.touch(manifestRef, entryTypeManifest); err != nil {
		return err
	}
	return nil
}

// Start starts the LRU eviction controller
func (lruec *LRUEvictionController) Start() error {
	lruec.Lock()
	defer lruec.Unlock()

	if !lruec.stopped {
		return fmt.Errorf("LRU eviction controller already started")
	}

	err := lruec.readState()
	if err != nil {
		return err
	}

	head, headOk := lruec.entries[entryNameHead]
	tail, tailOk := lruec.entries[entryNameTail]
	if !headOk || !tailOk {
		return fmt.Errorf("Lost head and/or tail entry when reading state")
	}
	lruec.head = head
	lruec.tail = tail

	dcontext.GetLogger(lruec.ctx).Infof("Starting cached object LRU eviction controller...")
	lruec.stopped = false

	// since we only checkpoint periodically, it is possible that we lose the latest state upon recovery if we encounter
	// a non-graceful shutdown. If this becomes a problem, we maybe should change `add` and `touch` to not have a strong
	// requirement on being consistent with the storage layer and instead just do `addOrTouch`.

	// Start a ticker to periodically save the entries index
	go func() {
		for {
			select {
			case <-lruec.saveTimer.C:
				lruec.Lock()
				if !lruec.indexDirty {
					lruec.Unlock()
					continue
				}

				err := lruec.writeState()
				if err != nil {
					dcontext.GetLogger(lruec.ctx).Errorf("Error writing LRU state: %s", err)
				} else {
					lruec.indexDirty = false
				}
				lruec.Unlock()

			case <-lruec.doneChan:
				return
			}
		}
	}()

	return nil
}

// mustGetEntry wraps around the map get for entries. It panics if entry is not found because this means a broken linked list
func (lruec *LRUEvictionController) mustGetEntry(key string) *lruEntry {
	entry, ok := lruec.entries[key]
	if !ok {
		panic("Unable to find entry %s, aborting due to broken linked list in LRU")
	}
	return entry
}

// add is the internal implementation called by AddBlob and AddManifest
func (lruec *LRUEvictionController) add(r reference.Canonical, eType int) error {
	entryKey := r.Digest().String()
	_, ok := lruec.entries[entryKey]
	if ok {
		return fmt.Errorf("Entry %s already exists in cache entries, inconsistency with storage", entryKey)
	}

	// Append entry back to tail of linked list
	tail := lruec.tail
	prev := lruec.mustGetEntry(tail.Prev)
	entry := &lruEntry{
		Key:        r.Digest().String(),
		EntryType:  eType,
		References: []string{r.Name()},
		Prev:       prev.Key,
		Next:       tail.Key,
	}

	dcontext.GetLogger(lruec.ctx).Infof("Adding lru entry for %s", entry.Key)
	prev.Next = entryKey
	tail.Prev = entryKey
	lruec.entries[entry.Key] = entry
	lruec.indexDirty = true

	if err := lruec.evict(); err != nil {
		return fmt.Errorf("Error evicting entries after inserting %s: %v", entryKey, err)
	}
	return nil
}

// touch is the internal implementation called by TouchBlob and TouchManifest
func (lruec *LRUEvictionController) touch(r reference.Canonical, eType int) error {
	entryKey := r.Digest().String()
	entry, ok := lruec.entries[entryKey]
	if !ok {
		return fmt.Errorf("Entry %s does not exist in cache entries, inconsistency with storage", entryKey)
	}
	if entry.EntryType != eType {
		return fmt.Errorf("Entry %s is expected to have entry type %d, got %d", entryKey, entry.EntryType, eType)
	}

	if !slices.Contains(entry.References, r.Name()) {
		entry.References = append(entry.References, r.Name())
		lruec.indexDirty = true
	}

	// If the entry is already the last entry, no need to move to last
	if entry.Next == entryNameTail {
		return nil
	}

	tail := lruec.tail
	prev := lruec.mustGetEntry(entry.Prev)
	next := lruec.mustGetEntry(entry.Next)
	newPrev := lruec.mustGetEntry(tail.Prev)

	dcontext.GetLogger(lruec.ctx).Infof("Touching lru entry for %s", entry.Key)

	// Remove entry's links in linked list
	prev.Next = entry.Next
	next.Prev = entry.Prev

	// Append entry back to tail of linked list
	entry.Prev = newPrev.Key
	entry.Next = tail.Key
	newPrev.Next = entryKey
	tail.Prev = entryKey

	lruec.indexDirty = true
	return nil
}

// Evict removes entries until size of the storage backend is below the specified limit
func (lruec *LRUEvictionController) evict() error {
	head := lruec.head
	tail := lruec.tail

	for {
		size, err := lruec.driver.Usage(lruec.ctx, "/")
		if err != nil {
			return fmt.Errorf("Could not get storagedriver usage at \"/\": %s", err)
		}

		if size <= lruec.limit {
			break
		}
		if head.Next == tail.Key {
			return fmt.Errorf("Cache eviction has evicted all entries but storage is still above limit")
		}
		entry := lruec.mustGetEntry(head.Next)

		// Delete each reference to the entry in the storage backend
		var f proxy.OnEvictFunc

		switch entry.EntryType {
		case entryTypeBlob:
			f = lruec.onBlobExpire
		case entryTypeManifest:
			f = lruec.onManifestExpire
		default:
			f = func(reference.Reference) error {
				return fmt.Errorf("lru entry type")
			}
		}
		for _, ref := range entry.References {
			// The left hand side is a Named reference, and the right hand side is the digest
			canonical, err := reference.Parse(ref + "@" + entry.Key)
			if err != nil {
				return fmt.Errorf("Error unpacking reference: %s", err)
			}
			if err := f(canonical); err != nil {
				return fmt.Errorf("Eviction error returned from OnEvict(%s): %s", entry.Key, err)
			}
		}

		next := lruec.mustGetEntry(entry.Next)

		dcontext.GetLogger(lruec.ctx).Infof("Evicting lru entry: %s", entry.Key)
		head.Next = entry.Next
		next.Prev = entry.Prev
		delete(lruec.entries, entry.Key)
		lruec.indexDirty = true
	}
	return nil
}

// Stop stops the LRU eviction controller.
func (lruec *LRUEvictionController) Stop() error {
	lruec.Lock()
	defer lruec.Unlock()

	err := lruec.writeState()
	if err != nil {
		err = fmt.Errorf("error writing LRU state: %w", err)
	}

	close(lruec.doneChan)
	lruec.saveTimer.Stop()
	lruec.stopped = true
	return err
}

func (lruec *LRUEvictionController) writeState() error {
	jsonBytes, err := json.Marshal(lruec.entries)
	if err != nil {
		return err
	}

	err = lruec.driver.PutContent(lruec.ctx, lruec.pathToStateFile, jsonBytes)
	if err != nil {
		return err
	}

	return nil
}

func (lruec *LRUEvictionController) readState() error {
	if _, err := lruec.driver.Stat(lruec.ctx, lruec.pathToStateFile); err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			return nil
		default:
			return err
		}
	}

	bytes, err := lruec.driver.GetContent(lruec.ctx, lruec.pathToStateFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, &lruec.entries)
	if err != nil {
		return err
	}

	return nil
}
