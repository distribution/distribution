package distribution

import (
	"fmt"
	"io"
	"os"
)

// ReadSeekCloser defines the common type for most readable blob content.
// Really, this is the io packages missing friend.
type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// NewReadSeekCloser returns a ReadSeekCloser implementation that dispatches
// calls to a ReaderAt. Most blob implementations can use this type if ReadAt
// is sufficiently performant. Note that while a closer method is provided,
// this will *not* attempt to close the underlying ReaderAt.
func NewReadSeekCloser(length int64, readerAt io.ReaderAt) ReadSeekCloser {
	return &readSeekCloser{
		ReaderAt: readerAt,
		length:   length,
	}
}

type readSeekCloser struct {
	io.ReaderAt
	offset int64
	length int64
	err    error
}

func (rsc *readSeekCloser) Read(p []byte) (n int, err error) {
	if rsc.err != nil {
		return 0, rsc.err
	}

	if rsc.offset >= rsc.length {
		return 0, io.EOF
	}

	n, err = rsc.ReadAt(p, rsc.offset)
	rsc.offset += int64(n)
	return n, err
}

func (rsc *readSeekCloser) Seek(offset int64, whence int) (int64, error) {
	if rsc.err != nil {
		return 0, rsc.err
	}

	var err error
	var abs int64

	switch whence {
	case os.SEEK_SET:
		abs = offset
	case os.SEEK_CUR:
		abs = rsc.offset + offset
	case os.SEEK_END:
		abs = rsc.length + offset
	default:
		return 0, fmt.Errorf("readseekcloser: invalid whence")
	}

	if abs < 0 {
		err = fmt.Errorf("readSeekCloser: cannot seek to negative position")
	} else {
		rsc.offset = abs
	}

	return int64(rsc.offset), err
}

func (rsc *readSeekCloser) Close() error {
	if rsc.err != nil {
		return rsc.err
	}

	rsc.err = fmt.Errorf("readSeekCloser: closed")
	return nil
}
