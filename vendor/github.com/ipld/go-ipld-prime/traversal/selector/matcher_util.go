package selector

import "io"

type readerat struct {
	io.ReadSeeker
}

// ReadAt provides the io.ReadAt method over a ReadSeeker.
// This implementation does not support concurrent calls to `ReadAt`,
// as specified by the ReaderAt interface, and so must only be used
// in non-concurrent use cases.
func (r readerat) ReadAt(p []byte, off int64) (n int, err error) {
	// TODO: consider keeping track of current offset.
	_, err = r.Seek(off, 0)
	if err != nil {
		return 0, err
	}
	return r.Read(p)
}
