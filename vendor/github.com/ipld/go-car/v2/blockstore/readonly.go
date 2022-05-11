package blockstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/index"
	"github.com/ipld/go-car/v2/internal/carv1"
	"github.com/ipld/go-car/v2/internal/carv1/util"
	internalio "github.com/ipld/go-car/v2/internal/io"
	"github.com/multiformats/go-multihash"
	"github.com/multiformats/go-varint"
	"golang.org/x/exp/mmap"
)

var _ blockstore.Blockstore = (*ReadOnly)(nil)

var (
	errZeroLengthSection = fmt.Errorf("zero-length carv2 section not allowed by default; see WithZeroLengthSectionAsEOF option")
	errReadOnly          = fmt.Errorf("called write method on a read-only carv2 blockstore")
	errClosed            = fmt.Errorf("cannot use a carv2 blockstore after closing")
)

// ReadOnly provides a read-only CAR Block Store.
type ReadOnly struct {
	// mu allows ReadWrite to be safe for concurrent use.
	// It's in ReadOnly so that read operations also grab read locks,
	// given that ReadWrite embeds ReadOnly for methods like Get and Has.
	//
	// The main fields guarded by the mutex are the index and the underlying writers.
	// For simplicity, the entirety of the blockstore methods grab the mutex.
	mu sync.RWMutex

	// When true, the blockstore has been closed via Close, Discard, or
	// Finalize, and must not be used. Any further blockstore method calls
	// will return errClosed to avoid panics or broken behavior.
	closed bool

	// The backing containing the data payload in CARv1 format.
	backing io.ReaderAt
	// The CARv1 content index.
	idx index.Index

	// If we called carv2.NewReaderMmap, remember to close it too.
	carv2Closer io.Closer

	opts carv2.Options
}

type contextKey string

const asyncErrHandlerKey contextKey = "asyncErrorHandlerKey"

// UseWholeCIDs is a read option which makes a CAR blockstore identify blocks by
// whole CIDs, and not just their multihashes. The default is to use
// multihashes, which matches the current semantics of go-ipfs-blockstore v1.
//
// Enabling this option affects a number of methods, including read-only ones:
//
// • Get, Has, and HasSize will only return a block
// only if the entire CID is present in the CAR file.
//
// • AllKeysChan will return the original whole CIDs, instead of with their
// multicodec set to "raw" to just provide multihashes.
//
// • If AllowDuplicatePuts isn't set,
// Put and PutMany will deduplicate by the whole CID,
// allowing different CIDs with equal multihashes.
//
// Note that this option only affects the blockstore, and is ignored by the root
// go-car/v2 package.
func UseWholeCIDs(enable bool) carv2.Option {
	return func(o *carv2.Options) {
		o.BlockstoreUseWholeCIDs = enable
	}
}

// NewReadOnly creates a new ReadOnly blockstore from the backing with a optional index as idx.
// This function accepts both CARv1 and CARv2 backing.
// The blockstore is instantiated with the given index if it is not nil.
//
// Otherwise:
// * For a CARv1 backing an index is generated.
// * For a CARv2 backing an index is only generated if Header.HasIndex returns false.
//
// There is no need to call ReadOnly.Close on instances returned by this function.
func NewReadOnly(backing io.ReaderAt, idx index.Index, opts ...carv2.Option) (*ReadOnly, error) {
	b := &ReadOnly{
		opts: carv2.ApplyOptions(opts...),
	}

	version, err := readVersion(backing)
	if err != nil {
		return nil, err
	}
	switch version {
	case 1:
		if idx == nil {
			if idx, err = generateIndex(backing, opts...); err != nil {
				return nil, err
			}
		}
		b.backing = backing
		b.idx = idx
		return b, nil
	case 2:
		v2r, err := carv2.NewReader(backing, opts...)
		if err != nil {
			return nil, err
		}
		if idx == nil {
			if v2r.Header.HasIndex() {
				idx, err = index.ReadFrom(v2r.IndexReader())
				if err != nil {
					return nil, err
				}
			} else if idx, err = generateIndex(v2r.DataReader(), opts...); err != nil {
				return nil, err
			}
		}
		b.backing = v2r.DataReader()
		b.idx = idx
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported car version: %v", version)
	}
}

func readVersion(at io.ReaderAt) (uint64, error) {
	var rr io.Reader
	switch r := at.(type) {
	case io.Reader:
		rr = r
	default:
		rr = internalio.NewOffsetReadSeeker(r, 0)
	}
	return carv2.ReadVersion(rr)
}

func generateIndex(at io.ReaderAt, opts ...carv2.Option) (index.Index, error) {
	var rs io.ReadSeeker
	switch r := at.(type) {
	case io.ReadSeeker:
		rs = r
		// The version may have been read from the given io.ReaderAt; therefore move back to the begining.
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	default:
		rs = internalio.NewOffsetReadSeeker(r, 0)
	}

	// Note, we do not set any write options so that all write options fall back onto defaults.
	return carv2.GenerateIndex(rs, opts...)
}

// OpenReadOnly opens a read-only blockstore from a CAR file (either v1 or v2), generating an index if it does not exist.
// Note, the generated index if the index does not exist is ephemeral and only stored in memory.
// See car.GenerateIndex and Index.Attach for persisting index onto a CAR file.
func OpenReadOnly(path string, opts ...carv2.Option) (*ReadOnly, error) {
	f, err := mmap.Open(path)
	if err != nil {
		return nil, err
	}

	robs, err := NewReadOnly(f, nil, opts...)
	if err != nil {
		return nil, err
	}
	robs.carv2Closer = f

	return robs, nil
}

func (b *ReadOnly) readBlock(idx int64) (cid.Cid, []byte, error) {
	bcid, data, err := util.ReadNode(internalio.NewOffsetReadSeeker(b.backing, idx), b.opts.ZeroLengthSectionAsEOF)
	return bcid, data, err
}

// DeleteBlock is unsupported and always errors.
func (b *ReadOnly) DeleteBlock(_ context.Context, _ cid.Cid) error {
	return errReadOnly
}

// Has indicates if the store contains a block that corresponds to the given key.
// This function always returns true for any given key with multihash.IDENTITY code.
func (b *ReadOnly) Has(ctx context.Context, key cid.Cid) (bool, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if _, ok, err := isIdentity(key); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return false, errClosed
	}

	var fnFound bool
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		uar := internalio.NewOffsetReadSeeker(b.backing, int64(offset))
		var err error
		_, err = varint.ReadUvarint(uar)
		if err != nil {
			fnErr = err
			return false
		}
		_, readCid, err := cid.CidFromReader(uar)
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			fnFound = readCid.Equals(key)
			return !fnFound // continue looking if we haven't found it
		} else {
			fnFound = bytes.Equal(readCid.Hash(), key.Hash())
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return fnFound, fnErr
}

// Get gets a block corresponding to the given key.
// This API will always return true if the given key has multihash.IDENTITY code.
func (b *ReadOnly) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if digest, ok, err := isIdentity(key); err != nil {
		return nil, err
	} else if ok {
		return blocks.NewBlockWithCid(digest, key)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, errClosed
	}

	var fnData []byte
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		readCid, data, err := b.readBlock(int64(offset))
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			if readCid.Equals(key) {
				fnData = data
				return false
			} else {
				return true // continue looking
			}
		} else {
			if bytes.Equal(readCid.Hash(), key.Hash()) {
				fnData = data
			}
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return nil, blockstore.ErrNotFound
	} else if err != nil {
		return nil, err
	} else if fnErr != nil {
		return nil, fnErr
	}
	if fnData == nil {
		return nil, blockstore.ErrNotFound
	}
	return blocks.NewBlockWithCid(fnData, key)
}

// GetSize gets the size of an item corresponding to the given key.
func (b *ReadOnly) GetSize(ctx context.Context, key cid.Cid) (int, error) {
	// Check if the given CID has multihash.IDENTITY code
	// Note, we do this without locking, since there is no shared information to lock for in order to perform the check.
	if digest, ok, err := isIdentity(key); err != nil {
		return 0, err
	} else if ok {
		return len(digest), nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return 0, errClosed
	}

	fnSize := -1
	var fnErr error
	err := b.idx.GetAll(key, func(offset uint64) bool {
		rdr := internalio.NewOffsetReadSeeker(b.backing, int64(offset))
		sectionLen, err := varint.ReadUvarint(rdr)
		if err != nil {
			fnErr = err
			return false
		}
		cidLen, readCid, err := cid.CidFromReader(rdr)
		if err != nil {
			fnErr = err
			return false
		}
		if b.opts.BlockstoreUseWholeCIDs {
			if readCid.Equals(key) {
				fnSize = int(sectionLen) - cidLen
				return false
			} else {
				return true // continue looking
			}
		} else {
			if bytes.Equal(readCid.Hash(), key.Hash()) {
				fnSize = int(sectionLen) - cidLen
			}
			return false
		}
	})
	if errors.Is(err, index.ErrNotFound) {
		return -1, blockstore.ErrNotFound
	} else if err != nil {
		return -1, err
	} else if fnErr != nil {
		return -1, fnErr
	}
	if fnSize == -1 {
		return -1, blockstore.ErrNotFound
	}
	return fnSize, nil
}

func isIdentity(key cid.Cid) (digest []byte, ok bool, err error) {
	dmh, err := multihash.Decode(key.Hash())
	if err != nil {
		return nil, false, err
	}
	ok = dmh.Code == multihash.IDENTITY
	digest = dmh.Digest
	return digest, ok, nil
}

// Put is not supported and always returns an error.
func (b *ReadOnly) Put(context.Context, blocks.Block) error {
	return errReadOnly
}

// PutMany is not supported and always returns an error.
func (b *ReadOnly) PutMany(context.Context, []blocks.Block) error {
	return errReadOnly
}

// WithAsyncErrorHandler returns a context with async error handling set to the given errHandler.
// Any errors that occur during asynchronous operations of AllKeysChan will be passed to the given
// handler.
func WithAsyncErrorHandler(ctx context.Context, errHandler func(error)) context.Context {
	return context.WithValue(ctx, asyncErrHandlerKey, errHandler)
}

// AllKeysChan returns the list of keys in the CAR data payload.
// If the ctx is constructed using WithAsyncErrorHandler any errors that occur during asynchronous
// retrieval of CIDs will be passed to the error handler function set in context.
// Otherwise, errors will terminate the asynchronous operation silently.
//
// See WithAsyncErrorHandler
func (b *ReadOnly) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	// We release the lock when the channel-sending goroutine stops.
	// Note that we can't use a deferred unlock here,
	// because if we return a nil error,
	// we only want to unlock once the async goroutine has stopped.
	b.mu.RLock()

	if b.closed {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, errClosed
	}

	// TODO we may use this walk for populating the index, and we need to be able to iterate keys in this way somewhere for index generation. In general though, when it's asked for all keys from a blockstore with an index, we should iterate through the index when possible rather than linear reads through the full car.
	rdr := internalio.NewOffsetReadSeeker(b.backing, 0)
	header, err := carv1.ReadHeader(rdr)
	if err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, fmt.Errorf("error reading car header: %w", err)
	}
	headerSize, err := carv1.HeaderSize(header)
	if err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, err
	}

	// TODO: document this choice of 5, or use simpler buffering like 0 or 1.
	ch := make(chan cid.Cid, 5)

	// Seek to the end of header.
	if _, err = rdr.Seek(int64(headerSize), io.SeekStart); err != nil {
		b.mu.RUnlock() // don't hold the mutex forever
		return nil, err
	}

	go func() {
		defer b.mu.RUnlock()
		defer close(ch)

		for {
			length, err := varint.ReadUvarint(rdr)
			if err != nil {
				if err != io.EOF {
					maybeReportError(ctx, err)
				}
				return
			}

			// Null padding; by default it's an error.
			if length == 0 {
				if b.opts.ZeroLengthSectionAsEOF {
					break
				} else {
					maybeReportError(ctx, errZeroLengthSection)
					return
				}
			}

			thisItemForNxt := rdr.Offset()
			_, c, err := cid.CidFromReader(rdr)
			if err != nil {
				maybeReportError(ctx, err)
				return
			}
			if _, err := rdr.Seek(thisItemForNxt+int64(length), io.SeekStart); err != nil {
				maybeReportError(ctx, err)
				return
			}

			// If we're just using multihashes, flatten to the "raw" codec.
			if !b.opts.BlockstoreUseWholeCIDs {
				c = cid.NewCidV1(cid.Raw, c.Hash())
			}

			select {
			case ch <- c:
			case <-ctx.Done():
				maybeReportError(ctx, ctx.Err())
				return
			}
		}
	}()
	return ch, nil
}

// maybeReportError checks if an error handler is present in context associated to the key
// asyncErrHandlerKey, and if preset it will pass the error to it.
func maybeReportError(ctx context.Context, err error) {
	value := ctx.Value(asyncErrHandlerKey)
	if eh, _ := value.(func(error)); eh != nil {
		eh(err)
	}
}

// HashOnRead is currently unimplemented; hashing on reads never happens.
func (b *ReadOnly) HashOnRead(bool) {
	// TODO: implement before the final release?
}

// Roots returns the root CIDs of the backing CAR.
func (b *ReadOnly) Roots() ([]cid.Cid, error) {
	header, err := carv1.ReadHeader(internalio.NewOffsetReadSeeker(b.backing, 0))
	if err != nil {
		return nil, fmt.Errorf("error reading car header: %w", err)
	}
	return header.Roots, nil
}

// Close closes the underlying reader if it was opened by OpenReadOnly.
// After this call, the blockstore can no longer be used.
//
// Note that this call may block if any blockstore operations are currently in
// progress, including an AllKeysChan that hasn't been fully consumed or cancelled.
func (b *ReadOnly) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.closeWithoutMutex()
}

func (b *ReadOnly) closeWithoutMutex() error {
	b.closed = true
	if b.carv2Closer != nil {
		return b.carv2Closer.Close()
	}
	return nil
}
