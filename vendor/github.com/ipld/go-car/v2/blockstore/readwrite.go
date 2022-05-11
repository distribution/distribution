package blockstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/index"
	"github.com/ipld/go-car/v2/internal/carv1"
	"github.com/ipld/go-car/v2/internal/carv1/util"
	internalio "github.com/ipld/go-car/v2/internal/io"
	"github.com/multiformats/go-varint"
)

var _ blockstore.Blockstore = (*ReadWrite)(nil)

// ReadWrite implements a blockstore that stores blocks in CARv2 format.
// Blocks put into the blockstore can be read back once they are successfully written.
// This implementation is preferable for a write-heavy workload.
// The blocks are written immediately on Put and PutAll calls, while the index is stored in memory
// and updated incrementally.
//
// The Finalize function must be called once the putting blocks are finished.
// Upon calling Finalize header is finalized and index is written out.
// Once finalized, all read and write calls to this blockstore will result in errors.
type ReadWrite struct {
	ronly ReadOnly

	f          *os.File
	dataWriter *internalio.OffsetWriteSeeker
	idx        *insertionIndex
	header     carv2.Header

	opts carv2.Options
}

// WriteAsCarV1 is a write option which makes a CAR blockstore write the output
// as a CARv1 only, with no CARv2 header or index. Indexing is used internally
// during write but is discarded upon finalization.
//
// Note that this option only affects the blockstore, and is ignored by the root
// go-car/v2 package.
func WriteAsCarV1(asCarV1 bool) carv2.Option {
	return func(o *carv2.Options) {
		o.WriteAsCarV1 = asCarV1
	}
}

// AllowDuplicatePuts is a write option which makes a CAR blockstore not
// deduplicate blocks in Put and PutMany. The default is to deduplicate,
// which matches the current semantics of go-ipfs-blockstore v1.
//
// Note that this option only affects the blockstore, and is ignored by the root
// go-car/v2 package.
func AllowDuplicatePuts(allow bool) carv2.Option {
	return func(o *carv2.Options) {
		o.BlockstoreAllowDuplicatePuts = allow
	}
}

// OpenReadWrite creates a new ReadWrite at the given path with a provided set of root CIDs and options.
//
// ReadWrite.Finalize must be called once putting and reading blocks are no longer needed.
// Upon calling ReadWrite.Finalize the CARv2 header and index are written out onto the file and the
// backing file is closed. Once finalized, all read and write calls to this blockstore will result
// in errors. Note, ReadWrite.Finalize must be called on an open instance regardless of whether any
// blocks were put or not.
//
// If a file at given path does not exist, the instantiation will write car.Pragma and data payload
// header (i.e. the inner CARv1 header) onto the file before returning.
//
// When the given path already exists, the blockstore will attempt to resume from it.
// On resumption the existing data sections in file are re-indexed, allowing the caller to continue
// putting any remaining blocks without having to re-ingest blocks for which previous ReadWrite.Put
// returned successfully.
//
// Resumption only works on files that were created by a previous instance of a ReadWrite
// blockstore. This means a file created as a result of a successful call to OpenReadWrite can be
// resumed from as long as write operations such as ReadWrite.Put, ReadWrite.PutMany returned
// successfully. On resumption the roots argument and WithDataPadding option must match the
// previous instantiation of ReadWrite blockstore that created the file. More explicitly, the file
// resuming from must:
//   1. start with a complete CARv2 car.Pragma.
//   2. contain a complete CARv1 data header with root CIDs matching the CIDs passed to the
//      constructor, starting at offset optionally padded by WithDataPadding, followed by zero or
//      more complete data sections. If any corrupt data sections are present the resumption will fail.
//      Note, if set previously, the blockstore must use the same WithDataPadding option as before,
//      since this option is used to locate the CARv1 data payload.
//
// Note, resumption should be used with WithCidDeduplication, so that blocks that are successfully
// written into the file are not re-written. Unless, the user explicitly wants duplicate blocks.
//
// Resuming from finalized files is allowed. However, resumption will regenerate the index
// regardless by scanning every existing block in file.
func OpenReadWrite(path string, roots []cid.Cid, opts ...carv2.Option) (*ReadWrite, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666) // TODO: Should the user be able to configure FileMode permissions?
	if err != nil {
		return nil, fmt.Errorf("could not open read/write file: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		// Note, we should not get a an os.ErrNotExist here because the flags used to open file includes os.O_CREATE
		return nil, err
	}
	// Try and resume by default if the file size is non-zero.
	resume := stat.Size() != 0
	// If construction of blockstore fails, make sure to close off the open file.
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	// Instantiate block store.
	// Set the header fileld before applying options since padding options may modify header.
	rwbs := &ReadWrite{
		f:      f,
		idx:    newInsertionIndex(),
		header: carv2.NewHeader(0),
		opts:   carv2.ApplyOptions(opts...),
	}
	rwbs.ronly.opts = rwbs.opts

	if p := rwbs.opts.DataPadding; p > 0 {
		rwbs.header = rwbs.header.WithDataPadding(p)
	}
	if p := rwbs.opts.IndexPadding; p > 0 {
		rwbs.header = rwbs.header.WithIndexPadding(p)
	}

	offset := int64(rwbs.header.DataOffset)
	if rwbs.opts.WriteAsCarV1 {
		offset = 0
	}
	rwbs.dataWriter = internalio.NewOffsetWriter(rwbs.f, offset)
	v1r := internalio.NewOffsetReadSeeker(rwbs.f, offset)
	rwbs.ronly.backing = v1r
	rwbs.ronly.idx = rwbs.idx
	rwbs.ronly.carv2Closer = rwbs.f

	if resume {
		if err = rwbs.resumeWithRoots(!rwbs.opts.WriteAsCarV1, roots); err != nil {
			return nil, err
		}
	} else {
		if err = rwbs.initWithRoots(!rwbs.opts.WriteAsCarV1, roots); err != nil {
			return nil, err
		}
	}

	return rwbs, nil
}

func (b *ReadWrite) initWithRoots(v2 bool, roots []cid.Cid) error {
	if v2 {
		if _, err := b.f.WriteAt(carv2.Pragma, 0); err != nil {
			return err
		}
	}
	return carv1.WriteHeader(&carv1.CarHeader{Roots: roots, Version: 1}, b.dataWriter)
}

func (b *ReadWrite) resumeWithRoots(v2 bool, roots []cid.Cid) error {
	// On resumption it is expected that the CARv2 Pragma, and the CARv1 header is successfully written.
	// Otherwise we cannot resume from the file.
	// Read pragma to assert if b.f is indeed a CARv2.
	version, err := carv2.ReadVersion(b.f)
	if err != nil {
		// The file is not a valid CAR file and cannot resume from it.
		// Or the write must have failed before pragma was written.
		return err
	}
	switch {
	case version == 1 && !v2:
	case version == 2 && v2:
	default:
		// The file is not the expected version and we cannot resume from it.
		return fmt.Errorf("cannot resume on CAR file with version %v", version)
	}

	var headerInFile carv2.Header

	if v2 {
		// Check if file was finalized by trying to read the CARv2 header.
		// We check because if finalized the CARv1 reader behaviour needs to be adjusted since
		// EOF will not signify end of CARv1 payload. i.e. index is most likely present.
		_, err = headerInFile.ReadFrom(internalio.NewOffsetReadSeeker(b.f, carv2.PragmaSize))

		// If reading CARv2 header succeeded, and CARv1 offset in header is not zero then the file is
		// most-likely finalized. Check padding and truncate the file to remove index.
		// Otherwise, carry on reading the v1 payload at offset determined from b.header.
		if err == nil && headerInFile.DataOffset != 0 {
			if headerInFile.DataOffset != b.header.DataOffset {
				// Assert that the padding on file matches the given WithDataPadding option.
				wantPadding := headerInFile.DataOffset - carv2.PragmaSize - carv2.HeaderSize
				gotPadding := b.header.DataOffset - carv2.PragmaSize - carv2.HeaderSize
				return fmt.Errorf(
					"cannot resume from file with mismatched CARv1 offset; "+
						"`WithDataPadding` option must match the padding on file. "+
						"Expected padding value of %v but got %v", wantPadding, gotPadding,
				)
			} else if headerInFile.DataSize == 0 {
				// If CARv1 size is zero, since CARv1 offset wasn't, then the CARv2 header was
				// most-likely partially written. Since we write the header last in Finalize then the
				// file most-likely contains the index and we cannot know where it starts, therefore
				// can't resume.
				return errors.New("corrupt CARv2 header; cannot resume from file")
			}
		}
	}

	// Use the given CARv1 padding to instantiate the CARv1 reader on file.
	v1r := internalio.NewOffsetReadSeeker(b.ronly.backing, 0)
	header, err := carv1.ReadHeader(v1r)
	if err != nil {
		// Cannot read the CARv1 header; the file is most likely corrupt.
		return fmt.Errorf("error reading car header: %w", err)
	}
	if !header.Matches(carv1.CarHeader{Roots: roots, Version: 1}) {
		// Cannot resume if version and root does not match.
		return errors.New("cannot resume on file with mismatching data header")
	}

	if headerInFile.DataOffset != 0 {
		// If header in file contains the size of car v1, then the index is most likely present.
		// Since we will need to re-generate the index, as the one in file is flattened, truncate
		// the file so that the Readonly.backing has the right set of bytes to deal with.
		// This effectively means resuming from a finalized file will wipe its index even if there
		// are no blocks put unless the user calls finalize.
		if err := b.f.Truncate(int64(headerInFile.DataOffset + headerInFile.DataSize)); err != nil {
			return err
		}
	}

	if v2 {
		// Now that CARv2 header is present on file, clear it to avoid incorrect size and offset in
		// header in case blocksotre is closed without finalization and is resumed from.
		if err := b.unfinalize(); err != nil {
			return fmt.Errorf("could not un-finalize: %w", err)
		}
	}

	// TODO See how we can reduce duplicate code here.
	// The code here comes from car.GenerateIndex.
	// Copied because we need to populate an insertindex, not a sorted index.
	// Producing a sorted index via generate, then converting it to insertindex is not possible.
	// Because Index interface does not expose internal records.
	// This may be done as part of https://github.com/ipld/go-car/issues/95

	offset, err := carv1.HeaderSize(header)
	if err != nil {
		return err
	}
	sectionOffset := int64(0)
	if sectionOffset, err = v1r.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}

	for {
		// Grab the length of the section.
		// Note that ReadUvarint wants a ByteReader.
		length, err := varint.ReadUvarint(v1r)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Null padding; by default it's an error.
		if length == 0 {
			if b.ronly.opts.ZeroLengthSectionAsEOF {
				break
			} else {
				return fmt.Errorf("carv1 null padding not allowed by default; see WithZeroLegthSectionAsEOF")
			}
		}

		// Grab the CID.
		n, c, err := cid.CidFromReader(v1r)
		if err != nil {
			return err
		}
		b.idx.insertNoReplace(c, uint64(sectionOffset))

		// Seek to the next section by skipping the block.
		// The section length includes the CID, so subtract it.
		if sectionOffset, err = v1r.Seek(int64(length)-int64(n), io.SeekCurrent); err != nil {
			return err
		}
	}
	// Seek to the end of last skipped block where the writer should resume writing.
	_, err = b.dataWriter.Seek(sectionOffset, io.SeekStart)
	return err
}

func (b *ReadWrite) unfinalize() error {
	_, err := new(carv2.Header).WriteTo(internalio.NewOffsetWriter(b.f, carv2.PragmaSize))
	return err
}

// Put puts a given block to the underlying datastore
func (b *ReadWrite) Put(ctx context.Context, blk blocks.Block) error {
	// PutMany already checks b.ronly.closed.
	return b.PutMany(ctx, []blocks.Block{blk})
}

// PutMany puts a slice of blocks at the same time using batching
// capabilities of the underlying datastore whenever possible.
func (b *ReadWrite) PutMany(ctx context.Context, blks []blocks.Block) error {
	b.ronly.mu.Lock()
	defer b.ronly.mu.Unlock()

	if b.ronly.closed {
		return errClosed
	}

	for _, bl := range blks {
		c := bl.Cid()

		// If StoreIdentityCIDs option is disabled then treat IDENTITY CIDs like IdStore.
		if !b.opts.StoreIdentityCIDs {
			// Check for IDENTITY CID. If IDENTITY, ignore and move to the next block.
			if _, ok, err := isIdentity(c); err != nil {
				return err
			} else if ok {
				continue
			}
		}

		// Check if its size is too big.
		// If larger than maximum allowed size, return error.
		// Note, we need to check this regardless of whether we have IDENTITY CID or not.
		// Since multhihash codes other than IDENTITY can result in large digests.
		cSize := uint64(len(c.Bytes()))
		if cSize > b.opts.MaxIndexCidSize {
			return &carv2.ErrCidTooLarge{MaxSize: b.opts.MaxIndexCidSize, CurrentSize: cSize}
		}

		if !b.opts.BlockstoreAllowDuplicatePuts {
			if b.ronly.opts.BlockstoreUseWholeCIDs && b.idx.hasExactCID(c) {
				continue // deduplicated by CID
			}
			if !b.ronly.opts.BlockstoreUseWholeCIDs {
				_, err := b.idx.Get(c)
				if err == nil {
					continue // deduplicated by hash
				}
			}
		}

		n := uint64(b.dataWriter.Position())
		if err := util.LdWrite(b.dataWriter, c.Bytes(), bl.RawData()); err != nil {
			return err
		}
		b.idx.insertNoReplace(c, n)
	}
	return nil
}

// Discard closes this blockstore without finalizing its header and index.
// After this call, the blockstore can no longer be used.
//
// Note that this call may block if any blockstore operations are currently in
// progress, including an AllKeysChan that hasn't been fully consumed or cancelled.
func (b *ReadWrite) Discard() {
	// Same semantics as ReadOnly.Close, including allowing duplicate calls.
	// The only difference is that our method is called Discard,
	// to further clarify that we're not properly finalizing and writing a
	// CARv2 file.
	b.ronly.Close()
}

// Finalize finalizes this blockstore by writing the CARv2 header, along with flattened index
// for more efficient subsequent read.
// After this call, the blockstore can no longer be used.
func (b *ReadWrite) Finalize() error {
	if b.opts.WriteAsCarV1 {
		// all blocks are already properly written to the CARv1 inner container and there's
		// no additional finalization required at the end of the file for a complete v1
		b.ronly.Close()
		return nil
	}

	b.ronly.mu.Lock()
	defer b.ronly.mu.Unlock()

	if b.ronly.closed {
		// Allow duplicate Finalize calls, just like Close.
		// Still error, just like ReadOnly.Close; it should be discarded.
		return fmt.Errorf("called Finalize on a closed blockstore")
	}

	// TODO check if add index option is set and don't write the index then set index offset to zero.
	b.header = b.header.WithDataSize(uint64(b.dataWriter.Position()))
	b.header.Characteristics.SetFullyIndexed(b.opts.StoreIdentityCIDs)

	// Note that we can't use b.Close here, as that tries to grab the same
	// mutex we're holding here.
	defer b.ronly.closeWithoutMutex()

	// TODO if index not needed don't bother flattening it.
	fi, err := b.idx.flatten(b.opts.IndexCodec)
	if err != nil {
		return err
	}
	if _, err := index.WriteTo(fi, internalio.NewOffsetWriter(b.f, int64(b.header.IndexOffset))); err != nil {
		return err
	}
	if _, err := b.header.WriteTo(internalio.NewOffsetWriter(b.f, carv2.PragmaSize)); err != nil {
		return err
	}

	if err := b.ronly.closeWithoutMutex(); err != nil {
		return err
	}
	return nil
}

func (b *ReadWrite) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.ronly.AllKeysChan(ctx)
}

func (b *ReadWrite) Has(ctx context.Context, key cid.Cid) (bool, error) {
	return b.ronly.Has(ctx, key)
}

func (b *ReadWrite) Get(ctx context.Context, key cid.Cid) (blocks.Block, error) {
	return b.ronly.Get(ctx, key)
}

func (b *ReadWrite) GetSize(ctx context.Context, key cid.Cid) (int, error) {
	return b.ronly.GetSize(ctx, key)
}

func (b *ReadWrite) DeleteBlock(_ context.Context, _ cid.Cid) error {
	return fmt.Errorf("ReadWrite blockstore does not support deleting blocks")
}

func (b *ReadWrite) HashOnRead(enable bool) {
	b.ronly.HashOnRead(enable)
}

func (b *ReadWrite) Roots() ([]cid.Cid, error) {
	return b.ronly.Roots()
}
