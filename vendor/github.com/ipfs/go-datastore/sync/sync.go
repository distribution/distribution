package sync

import (
	"context"
	"sync"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
)

// MutexDatastore contains a child datastore and a mutex.
// used for coarse sync
type MutexDatastore struct {
	sync.RWMutex

	child ds.Datastore
}

var _ ds.Datastore = (*MutexDatastore)(nil)
var _ ds.Batching = (*MutexDatastore)(nil)
var _ ds.Shim = (*MutexDatastore)(nil)
var _ ds.PersistentDatastore = (*MutexDatastore)(nil)
var _ ds.CheckedDatastore = (*MutexDatastore)(nil)
var _ ds.ScrubbedDatastore = (*MutexDatastore)(nil)
var _ ds.GCDatastore = (*MutexDatastore)(nil)

// MutexWrap constructs a datastore with a coarse lock around the entire
// datastore, for every single operation.
func MutexWrap(d ds.Datastore) *MutexDatastore {
	return &MutexDatastore{child: d}
}

// Children implements Shim
func (d *MutexDatastore) Children() []ds.Datastore {
	return []ds.Datastore{d.child}
}

// Put implements Datastore.Put
func (d *MutexDatastore) Put(ctx context.Context, key ds.Key, value []byte) (err error) {
	d.Lock()
	defer d.Unlock()
	return d.child.Put(ctx, key, value)
}

// Sync implements Datastore.Sync
func (d *MutexDatastore) Sync(ctx context.Context, prefix ds.Key) error {
	d.Lock()
	defer d.Unlock()
	return d.child.Sync(ctx, prefix)
}

// Get implements Datastore.Get
func (d *MutexDatastore) Get(ctx context.Context, key ds.Key) (value []byte, err error) {
	d.RLock()
	defer d.RUnlock()
	return d.child.Get(ctx, key)
}

// Has implements Datastore.Has
func (d *MutexDatastore) Has(ctx context.Context, key ds.Key) (exists bool, err error) {
	d.RLock()
	defer d.RUnlock()
	return d.child.Has(ctx, key)
}

// GetSize implements Datastore.GetSize
func (d *MutexDatastore) GetSize(ctx context.Context, key ds.Key) (size int, err error) {
	d.RLock()
	defer d.RUnlock()
	return d.child.GetSize(ctx, key)
}

// Delete implements Datastore.Delete
func (d *MutexDatastore) Delete(ctx context.Context, key ds.Key) (err error) {
	d.Lock()
	defer d.Unlock()
	return d.child.Delete(ctx, key)
}

// Query implements Datastore.Query
func (d *MutexDatastore) Query(ctx context.Context, q dsq.Query) (dsq.Results, error) {
	d.RLock()
	defer d.RUnlock()

	// Apply the entire query while locked. Non-sync datastores may not
	// allow concurrent queries.

	results, err := d.child.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	entries, err1 := results.Rest()
	err2 := results.Close()
	switch {
	case err1 != nil:
		return nil, err1
	case err2 != nil:
		return nil, err2
	}
	return dsq.ResultsWithEntries(q, entries), nil
}

func (d *MutexDatastore) Batch(ctx context.Context) (ds.Batch, error) {
	d.RLock()
	defer d.RUnlock()
	bds, ok := d.child.(ds.Batching)
	if !ok {
		return nil, ds.ErrBatchUnsupported
	}

	b, err := bds.Batch(ctx)
	if err != nil {
		return nil, err
	}
	return &syncBatch{
		batch: b,
		mds:   d,
	}, nil
}

func (d *MutexDatastore) Close() error {
	d.RWMutex.Lock()
	defer d.RWMutex.Unlock()
	return d.child.Close()
}

// DiskUsage implements the PersistentDatastore interface.
func (d *MutexDatastore) DiskUsage(ctx context.Context) (uint64, error) {
	d.RLock()
	defer d.RUnlock()
	return ds.DiskUsage(ctx, d.child)
}

type syncBatch struct {
	batch ds.Batch
	mds   *MutexDatastore
}

var _ ds.Batch = (*syncBatch)(nil)

func (b *syncBatch) Put(ctx context.Context, key ds.Key, val []byte) error {
	b.mds.Lock()
	defer b.mds.Unlock()
	return b.batch.Put(ctx, key, val)
}

func (b *syncBatch) Delete(ctx context.Context, key ds.Key) error {
	b.mds.Lock()
	defer b.mds.Unlock()
	return b.batch.Delete(ctx, key)
}

func (b *syncBatch) Commit(ctx context.Context) error {
	b.mds.Lock()
	defer b.mds.Unlock()
	return b.batch.Commit(ctx)
}

func (d *MutexDatastore) Check(ctx context.Context) error {
	if c, ok := d.child.(ds.CheckedDatastore); ok {
		d.RWMutex.Lock()
		defer d.RWMutex.Unlock()
		return c.Check(ctx)
	}
	return nil
}

func (d *MutexDatastore) Scrub(ctx context.Context) error {
	if c, ok := d.child.(ds.ScrubbedDatastore); ok {
		d.RWMutex.Lock()
		defer d.RWMutex.Unlock()
		return c.Scrub(ctx)
	}
	return nil
}

func (d *MutexDatastore) CollectGarbage(ctx context.Context) error {
	if c, ok := d.child.(ds.GCDatastore); ok {
		d.RWMutex.Lock()
		defer d.RWMutex.Unlock()
		return c.CollectGarbage(ctx)
	}
	return nil
}
