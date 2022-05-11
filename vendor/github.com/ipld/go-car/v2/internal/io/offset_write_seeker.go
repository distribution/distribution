package io

import "io"

var (
	_ io.Writer      = (*OffsetWriteSeeker)(nil)
	_ io.WriteSeeker = (*OffsetWriteSeeker)(nil)
)

type OffsetWriteSeeker struct {
	w      io.WriterAt
	base   int64
	offset int64
}

func NewOffsetWriter(w io.WriterAt, off int64) *OffsetWriteSeeker {
	return &OffsetWriteSeeker{w, off, off}
}

func (ow *OffsetWriteSeeker) Write(b []byte) (n int, err error) {
	n, err = ow.w.WriteAt(b, ow.offset)
	ow.offset += int64(n)
	return
}

func (ow *OffsetWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		ow.offset = offset + ow.base
	case io.SeekCurrent:
		ow.offset += offset
	case io.SeekEnd:
		panic("unsupported whence: SeekEnd")
	}
	return ow.Position(), nil
}

// Position returns the current position of this writer relative to the initial offset, i.e. the number of bytes written.
func (ow *OffsetWriteSeeker) Position() int64 {
	return ow.offset - ow.base
}
