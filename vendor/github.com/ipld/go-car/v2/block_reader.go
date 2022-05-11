package car

import (
	"fmt"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/internal/carv1"
	"github.com/ipld/go-car/v2/internal/carv1/util"
	internalio "github.com/ipld/go-car/v2/internal/io"
)

// BlockReader facilitates iteration over CAR blocks for both CARv1 and CARv2.
// See NewBlockReader
type BlockReader struct {
	// The detected version of the CAR payload.
	Version uint64
	// The roots of the CAR payload. May be empty.
	Roots []cid.Cid

	// Used internally only, by BlockReader.Next during iteration over blocks.
	r    io.Reader
	opts Options
}

// NewBlockReader instantiates a new BlockReader facilitating iteration over blocks in CARv1 or
// CARv2 payload. Upon instantiation, the version is automatically detected and exposed via
// BlockReader.Version. The root CIDs of the CAR payload are exposed via BlockReader.Roots
//
// See BlockReader.Next
func NewBlockReader(r io.Reader, opts ...Option) (*BlockReader, error) {
	// Read CARv1 header or CARv2 pragma.
	// Both are a valid CARv1 header, therefore are read as such.
	pragmaOrV1Header, err := carv1.ReadHeader(r)
	if err != nil {
		return nil, err
	}

	// Populate the block reader version and options.
	br := &BlockReader{
		Version: pragmaOrV1Header.Version,
		opts:    ApplyOptions(opts...),
	}

	// Expect either version 1 or 2.
	switch br.Version {
	case 1:
		// If version is 1, r represents a CARv1.
		// Simply populate br.Roots and br.r without modifying r.
		br.Roots = pragmaOrV1Header.Roots
		br.r = r
	case 2:
		// If the version is 2:
		//  1. Read CARv2 specific header to locate the inner CARv1 data payload offset and size.
		//  2. Skip to the beginning of the inner CARv1 data payload.
		//  3. Re-initialize r as a LimitReader, limited to the size of the inner CARv1 payload.
		//  4. Read the header of inner CARv1 data payload via r to populate br.Roots.

		// Read CARv2-specific header.
		v2h := Header{}
		if _, err := v2h.ReadFrom(r); err != nil {
			return nil, err
		}
		// Assert the data payload offset validity.
		// It must be at least 51 (<CARv2Pragma> + <CARv2Header>).
		dataOffset := int64(v2h.DataOffset)
		if dataOffset < PragmaSize+HeaderSize {
			return nil, fmt.Errorf("invalid data payload offset: %v", dataOffset)
		}
		// Assert the data size validity.
		// It must be larger than zero.
		// Technically, it should be at least 11 bytes (i.e. a valid CARv1 header with no roots) but
		// we let further parsing of the header to signal invalid data payload header.
		dataSize := int64(v2h.DataSize)
		if dataSize <= 0 {
			return nil, fmt.Errorf("invalid data payload size: %v", dataSize)
		}

		// Skip to the beginning of inner CARv1 data payload.
		// Note, at this point the pragma and CARv1 header have been read.
		// An io.ReadSeeker is opportunistically constructed from the given io.Reader r.
		// The constructor does not take an initial offset, so we use Seek in io.SeekCurrent to
		// fast forward to the beginning of data payload by subtracting pragma and header size from
		// dataOffset.
		rs := internalio.ToByteReadSeeker(r)
		if _, err := rs.Seek(dataOffset-PragmaSize-HeaderSize, io.SeekCurrent); err != nil {
			return nil, err
		}

		// Set br.r to a LimitReader reading from r limited to dataSize.
		br.r = io.LimitReader(r, dataSize)

		// Populate br.Roots by reading the inner CARv1 data payload header.
		header, err := carv1.ReadHeader(br.r)
		if err != nil {
			return nil, err
		}
		// Assert that the data payload header is exactly 1, i.e. the header represents a CARv1.
		if header.Version != 1 {
			return nil, fmt.Errorf("invalid data payload header version; expected 1, got %v", header.Version)
		}
		br.Roots = header.Roots
	default:
		// Otherwise, error out with invalid version since only versions 1 or 2 are expected.
		return nil, fmt.Errorf("invalid car version: %d", br.Version)
	}
	return br, nil
}

// Next iterates over blocks in the underlying CAR payload with an io.EOF error indicating the end
// is reached. Note, this function is forward-only; once the end has been reached it will always
// return io.EOF.
//
// When the payload represents a CARv1 the BlockReader.Next simply iterates over blocks until it
// reaches the end of the underlying io.Reader stream.
//
// As for CARv2 payload, the underlying io.Reader is read only up to the end of the last block.
// Note, in a case where ZeroLengthSectionAsEOF Option is enabled, io.EOF is returned
// immediately upon encountering a zero-length section without reading any further bytes from the
// underlying io.Reader.
func (br *BlockReader) Next() (blocks.Block, error) {
	c, data, err := util.ReadNode(br.r, br.opts.ZeroLengthSectionAsEOF)
	if err != nil {
		return nil, err
	}

	hashed, err := c.Prefix().Sum(data)
	if err != nil {
		return nil, err
	}

	if !hashed.Equals(c) {
		return nil, fmt.Errorf("mismatch in content integrity, name: %s, data: %s", c, hashed)
	}

	return blocks.NewBlockWithCid(data, c)
}
