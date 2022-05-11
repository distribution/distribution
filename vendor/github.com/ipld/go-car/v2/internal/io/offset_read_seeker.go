package io

import "io"

var (
	_ io.ReaderAt   = (*OffsetReadSeeker)(nil)
	_ io.ReadSeeker = (*OffsetReadSeeker)(nil)
)

// OffsetReadSeeker implements Read, and ReadAt on a section
// of an underlying io.ReaderAt.
// The main difference between io.SectionReader and OffsetReadSeeker is that
// NewOffsetReadSeeker does not require the user to know the number of readable bytes.
//
// It also partially implements Seek, where the implementation panics if io.SeekEnd is passed.
// This is because, OffsetReadSeeker does not know the end of the file therefore cannot seek relative
// to it.
type OffsetReadSeeker struct {
	r    io.ReaderAt
	base int64
	off  int64
}

// NewOffsetReadSeeker returns an OffsetReadSeeker that reads from r
// starting offset offset off and stops with io.EOF when r reaches its end.
// The Seek function will panic if whence io.SeekEnd is passed.
func NewOffsetReadSeeker(r io.ReaderAt, off int64) *OffsetReadSeeker {
	return &OffsetReadSeeker{r, off, off}
}

func (o *OffsetReadSeeker) Read(p []byte) (n int, err error) {
	n, err = o.r.ReadAt(p, o.off)
	o.off += int64(n)
	return
}

func (o *OffsetReadSeeker) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, io.EOF
	}
	off += o.base
	return o.r.ReadAt(p, off)
}

func (o *OffsetReadSeeker) ReadByte() (byte, error) {
	b := []byte{0}
	_, err := o.Read(b)
	return b[0], err
}

func (o *OffsetReadSeeker) Offset() int64 {
	return o.off
}

func (o *OffsetReadSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		o.off = offset + o.base
	case io.SeekCurrent:
		o.off += offset
	case io.SeekEnd:
		panic("unsupported whence: SeekEnd")
	}
	return o.Position(), nil
}

// Position returns the current position of this reader relative to the initial offset.
func (o *OffsetReadSeeker) Position() int64 {
	return o.off - o.base
}
