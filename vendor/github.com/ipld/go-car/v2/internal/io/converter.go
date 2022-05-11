package io

import (
	"io"
	"io/ioutil"
	"sync"
)

var (
	_ io.ByteReader = (*readerPlusByte)(nil)
	_ io.ByteReader = (*readSeekerPlusByte)(nil)
	_ io.ByteReader = (*discardingReadSeekerPlusByte)(nil)
	_ io.ReadSeeker = (*discardingReadSeekerPlusByte)(nil)
	_ io.ReaderAt   = (*readSeekerAt)(nil)
)

type (
	readerPlusByte struct {
		io.Reader

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	readSeekerPlusByte struct {
		io.ReadSeeker

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	discardingReadSeekerPlusByte struct {
		io.Reader
		offset int64

		byteBuf [1]byte // escapes via io.Reader.Read; preallocate
	}

	ByteReadSeeker interface {
		io.ReadSeeker
		io.ByteReader
	}

	readSeekerAt struct {
		rs io.ReadSeeker
		mu sync.Mutex
	}
)

func ToByteReader(r io.Reader) io.ByteReader {
	if br, ok := r.(io.ByteReader); ok {
		return br
	}
	return &readerPlusByte{Reader: r}
}

func ToByteReadSeeker(r io.Reader) ByteReadSeeker {
	if brs, ok := r.(ByteReadSeeker); ok {
		return brs
	}
	if rs, ok := r.(io.ReadSeeker); ok {
		return &readSeekerPlusByte{ReadSeeker: rs}
	}
	return &discardingReadSeekerPlusByte{Reader: r}
}

func ToReaderAt(rs io.ReadSeeker) io.ReaderAt {
	if ra, ok := rs.(io.ReaderAt); ok {
		return ra
	}
	return &readSeekerAt{rs: rs}
}

func (rb *readerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(rb, rb.byteBuf[:])
	return rb.byteBuf[0], err
}

func (rsb *readSeekerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(rsb, rsb.byteBuf[:])
	return rsb.byteBuf[0], err
}

func (drsb *discardingReadSeekerPlusByte) ReadByte() (byte, error) {
	_, err := io.ReadFull(drsb, drsb.byteBuf[:])
	return drsb.byteBuf[0], err
}

func (drsb *discardingReadSeekerPlusByte) Read(p []byte) (read int, err error) {
	read, err = drsb.Reader.Read(p)
	drsb.offset += int64(read)
	return
}

func (drsb *discardingReadSeekerPlusByte) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		n := offset - drsb.offset
		if n < 0 {
			panic("unsupported rewind via whence: io.SeekStart")
		}
		_, err := io.CopyN(ioutil.Discard, drsb, n)
		return drsb.offset, err
	case io.SeekCurrent:
		_, err := io.CopyN(ioutil.Discard, drsb, offset)
		return drsb.offset, err
	default:
		panic("unsupported whence: io.SeekEnd")
	}
}

func (rsa *readSeekerAt) ReadAt(p []byte, off int64) (n int, err error) {
	rsa.mu.Lock()
	defer rsa.mu.Unlock()
	if _, err := rsa.rs.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return rsa.rs.Read(p)
}
