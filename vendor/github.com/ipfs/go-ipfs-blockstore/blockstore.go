// Package blockstore implements a thin wrapper over a datastore, giving a
// clean interface for Getting and Putting block objects.
package blockstore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dsns "github.com/ipfs/go-datastore/namespace"
	dsq "github.com/ipfs/go-datastore/query"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	logging "github.com/ipfs/go-log"
	uatomic "go.uber.org/atomic"
)

var log = logging.Logger("blockstore")

// BlockPrefix namespaces blockstore datastores
var BlockPrefix = ds.NewKey("blocks")

// ErrHashMismatch is an error returned when the hash of a block
// is different than expected.
var ErrHashMismatch = errors.New("block in storage has different hash than requested")

// ErrNotFound is an error returned when a block is not found.
var ErrNotFound = errors.New("blockstore: block not found")

// Blockstore wraps a Datastore block-centered methods and provides a layer
// of abstraction which allows to add different caching strategies.
type Blockstore interface {
	DeleteBlock(context.Context, cid.Cid) error
	Has(context.Context, cid.Cid) (bool, error)
	Get(context.Context, cid.Cid) (blocks.Block, error)

	// GetSize returns the CIDs mapped BlockSize
	GetSize(context.Context, cid.Cid) (int, error)

	// Put puts a given block to the underlying datastore
	Put(context.Context, blocks.Block) error

	// PutMany puts a slice of blocks at the same time using batching
	// capabilities of the underlying datastore whenever possible.
	PutMany(context.Context, []blocks.Block) error

	// AllKeysChan returns a channel from which
	// the CIDs in the Blockstore can be read. It should respect
	// the given context, closing the channel if it becomes Done.
	AllKeysChan(ctx context.Context) (<-chan cid.Cid, error)

	// HashOnRead specifies if every read block should be
	// rehashed to make sure it matches its CID.
	HashOnRead(enabled bool)
}

// Viewer can be implemented by blockstores that offer zero-copy access to
// values.
//
// Callers of View must not mutate or retain the byte slice, as it could be
// an mmapped memory region, or a pooled byte buffer.
//
// View is especially suitable for deserialising in place.
//
// The callback will only be called iff the query operation is successful (and
// the block is found); otherwise, the error will be propagated. Errors returned
// by the callback will be propagated as well.
type Viewer interface {
	View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error
}

// GCLocker abstract functionality to lock a blockstore when performing
// garbage-collection operations.
type GCLocker interface {
	// GCLock locks the blockstore for garbage collection. No operations
	// that expect to finish with a pin should ocurr simultaneously.
	// Reading during GC is safe, and requires no lock.
	GCLock(context.Context) Unlocker

	// PinLock locks the blockstore for sequences of puts expected to finish
	// with a pin (before GC). Multiple put->pin sequences can write through
	// at the same time, but no GC should happen simulatenously.
	// Reading during Pinning is safe, and requires no lock.
	PinLock(context.Context) Unlocker

	// GcRequested returns true if GCLock has been called and is waiting to
	// take the lock
	GCRequested(context.Context) bool
}

// GCBlockstore is a blockstore that can safely run garbage-collection
// operations.
type GCBlockstore interface {
	Blockstore
	GCLocker
}

// NewGCBlockstore returns a default implementation of GCBlockstore
// using the given Blockstore and GCLocker.
func NewGCBlockstore(bs Blockstore, gcl GCLocker) GCBlockstore {
	return gcBlockstore{bs, gcl}
}

type gcBlockstore struct {
	Blockstore
	GCLocker
}

// NewBlockstore returns a default Blockstore implementation
// using the provided datastore.Batching backend.
func NewBlockstore(d ds.Batching) Blockstore {
	var dsb ds.Batching
	dd := dsns.Wrap(d, BlockPrefix)
	dsb = dd
	return &blockstore{
		datastore: dsb,
		rehash:    uatomic.NewBool(false),
	}
}

// NewBlockstoreNoPrefix returns a default Blockstore implementation
// using the provided datastore.Batching backend.
// This constructor does not modify input keys in any way
func NewBlockstoreNoPrefix(d ds.Batching) Blockstore {
	return &blockstore{
		datastore: d,
		rehash:    uatomic.NewBool(false),
	}
}

type blockstore struct {
	datastore ds.Batching

	rehash *uatomic.Bool
}

func (bs *blockstore) HashOnRead(enabled bool) {
	bs.rehash.Store(enabled)
}

func (bs *blockstore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	if !k.Defined() {
		log.Error("undefined cid in blockstore")
		return nil, ErrNotFound
	}
	bdata, err := bs.datastore.Get(ctx, dshelp.MultihashToDsKey(k.Hash()))
	if err == ds.ErrNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bs.rehash.Load() {
		rbcid, err := k.Prefix().Sum(bdata)
		if err != nil {
			return nil, err
		}

		if !rbcid.Equals(k) {
			return nil, ErrHashMismatch
		}

		return blocks.NewBlockWithCid(bdata, rbcid)
	}
	return blocks.NewBlockWithCid(bdata, k)
}

func (bs *blockstore) Put(ctx context.Context, block blocks.Block) error {
	k := dshelp.MultihashToDsKey(block.Cid().Hash())

	// Has is cheaper than Put, so see if we already have it
	exists, err := bs.datastore.Has(ctx, k)
	if err == nil && exists {
		return nil // already stored.
	}
	return bs.datastore.Put(ctx, k, block.RawData())
}

func (bs *blockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	t, err := bs.datastore.Batch(ctx)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		k := dshelp.MultihashToDsKey(b.Cid().Hash())
		exists, err := bs.datastore.Has(ctx, k)
		if err == nil && exists {
			continue
		}

		err = t.Put(ctx, k, b.RawData())
		if err != nil {
			return err
		}
	}
	return t.Commit(ctx)
}

func (bs *blockstore) Has(ctx context.Context, k cid.Cid) (bool, error) {
	return bs.datastore.Has(ctx, dshelp.MultihashToDsKey(k.Hash()))
}

func (bs *blockstore) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	size, err := bs.datastore.GetSize(ctx, dshelp.MultihashToDsKey(k.Hash()))
	if err == ds.ErrNotFound {
		return -1, ErrNotFound
	}
	return size, err
}

func (bs *blockstore) DeleteBlock(ctx context.Context, k cid.Cid) error {
	return bs.datastore.Delete(ctx, dshelp.MultihashToDsKey(k.Hash()))
}

// AllKeysChan runs a query for keys from the blockstore.
// this is very simplistic, in the future, take dsq.Query as a param?
//
// AllKeysChan respects context.
func (bs *blockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {

	// KeysOnly, because that would be _a lot_ of data.
	q := dsq.Query{KeysOnly: true}
	res, err := bs.datastore.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	output := make(chan cid.Cid, dsq.KeysOnlyBufSize)
	go func() {
		defer func() {
			res.Close() // ensure exit (signals early exit, too)
			close(output)
		}()

		for {
			e, ok := res.NextSync()
			if !ok {
				return
			}
			if e.Error != nil {
				log.Errorf("blockstore.AllKeysChan got err: %s", e.Error)
				return
			}

			// need to convert to key.Key using key.KeyFromDsKey.
			bk, err := dshelp.BinaryFromDsKey(ds.RawKey(e.Key))
			if err != nil {
				log.Warningf("error parsing key from binary: %s", err)
				continue
			}
			k := cid.NewCidV1(cid.Raw, bk)
			select {
			case <-ctx.Done():
				return
			case output <- k:
			}
		}
	}()

	return output, nil
}

// NewGCLocker returns a default implementation of
// GCLocker using standard [RW] mutexes.
func NewGCLocker() GCLocker {
	return &gclocker{}
}

type gclocker struct {
	lk    sync.RWMutex
	gcreq int32
}

// Unlocker represents an object which can Unlock
// something.
type Unlocker interface {
	Unlock(context.Context)
}

type unlocker struct {
	unlock func()
}

func (u *unlocker) Unlock(_ context.Context) {
	u.unlock()
	u.unlock = nil // ensure its not called twice
}

func (bs *gclocker) GCLock(_ context.Context) Unlocker {
	atomic.AddInt32(&bs.gcreq, 1)
	bs.lk.Lock()
	atomic.AddInt32(&bs.gcreq, -1)
	return &unlocker{bs.lk.Unlock}
}

func (bs *gclocker) PinLock(_ context.Context) Unlocker {
	bs.lk.RLock()
	return &unlocker{bs.lk.RUnlock}
}

func (bs *gclocker) GCRequested(_ context.Context) bool {
	return atomic.LoadInt32(&bs.gcreq) > 0
}
