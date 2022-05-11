package car

import (
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/internal/carv1"
	internalio "github.com/ipld/go-car/v2/internal/io"
	"golang.org/x/exp/mmap"
)

// Reader represents a reader of CARv2.
type Reader struct {
	Header  Header
	Version uint64
	r       io.ReaderAt
	roots   []cid.Cid
	opts    Options
	closer  io.Closer
}

// OpenReader is a wrapper for NewReader which opens the file at path.
func OpenReader(path string, opts ...Option) (*Reader, error) {
	f, err := mmap.Open(path)
	if err != nil {
		return nil, err
	}

	r, err := NewReader(f, opts...)
	if err != nil {
		return nil, err
	}

	r.closer = f
	return r, nil
}

// NewReader constructs a new reader that reads either CARv1 or CARv2 from the given r.
// Upon instantiation, the reader inspects the payload and provides appropriate read operations
// for both CARv1 and CARv2.
//
// Note that any other version other than 1 or 2 will result in an error. The caller may use
// Reader.Version to get the actual version r represents. In the case where r represents a CARv1
// Reader.Header will not be populated and is left as zero-valued.
func NewReader(r io.ReaderAt, opts ...Option) (*Reader, error) {
	cr := &Reader{
		r: r,
	}
	cr.opts = ApplyOptions(opts...)

	or := internalio.NewOffsetReadSeeker(r, 0)
	var err error
	cr.Version, err = ReadVersion(or)
	if err != nil {
		return nil, err
	}

	if cr.Version != 1 && cr.Version != 2 {
		return nil, fmt.Errorf("invalid car version: %d", cr.Version)
	}

	if cr.Version == 2 {
		if err := cr.readV2Header(); err != nil {
			return nil, err
		}
	}

	return cr, nil
}

// Roots returns the root CIDs.
// The root CIDs are extracted lazily from the data payload header.
func (r *Reader) Roots() ([]cid.Cid, error) {
	if r.roots != nil {
		return r.roots, nil
	}
	header, err := carv1.ReadHeader(r.DataReader())
	if err != nil {
		return nil, err
	}
	r.roots = header.Roots
	return r.roots, nil
}

func (r *Reader) readV2Header() (err error) {
	headerSection := io.NewSectionReader(r.r, PragmaSize, HeaderSize)
	_, err = r.Header.ReadFrom(headerSection)
	return
}

// SectionReader implements both io.ReadSeeker and io.ReaderAt.
// It is the interface version of io.SectionReader, but note that the
// implementation is not guaranteed to be an io.SectionReader.
type SectionReader interface {
	io.Reader
	io.Seeker
	io.ReaderAt
}

// DataReader provides a reader containing the data payload in CARv1 format.
func (r *Reader) DataReader() SectionReader {
	if r.Version == 2 {
		return io.NewSectionReader(r.r, int64(r.Header.DataOffset), int64(r.Header.DataSize))
	}
	return internalio.NewOffsetReadSeeker(r.r, 0)
}

// IndexReader provides an io.Reader containing the index for the data payload if the index is
// present. Otherwise, returns nil.
// Note, this function will always return nil if the backing payload represents a CARv1.
func (r *Reader) IndexReader() io.Reader {
	if r.Version == 1 || !r.Header.HasIndex() {
		return nil
	}
	return internalio.NewOffsetReadSeeker(r.r, int64(r.Header.IndexOffset))
}

// Close closes the underlying reader if it was opened by OpenReader.
func (r *Reader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

// ReadVersion reads the version from the pragma.
// This function accepts both CARv1 and CARv2 payloads.
func ReadVersion(r io.Reader) (uint64, error) {
	header, err := carv1.ReadHeader(r)
	if err != nil {
		return 0, err
	}
	return header.Version, nil
}
