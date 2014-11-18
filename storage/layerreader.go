package storage

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

// layerReadSeeker implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	layerStore *layerStore
	rc         io.ReadCloser
	brd        *bufio.Reader

	name      string // repo name of this layer
	tarSum    string
	path      string
	createdAt time.Time

	// offset is the current read offset
	offset int64

	// size is the total layer size, if available.
	size int64

	closedErr error // terminal error, if set, reader is closed
}

var _ Layer = &layerReader{}

func (lrs *layerReader) Name() string {
	return lrs.name
}

func (lrs *layerReader) TarSum() string {
	return lrs.tarSum
}

func (lrs *layerReader) CreatedAt() time.Time {
	return lrs.createdAt
}

func (lrs *layerReader) Read(p []byte) (n int, err error) {
	if err := lrs.closed(); err != nil {
		return 0, err
	}

	rd, err := lrs.reader()
	if err != nil {
		return 0, err
	}

	n, err = rd.Read(p)
	lrs.offset += int64(n)

	// Simulate io.EOR error if we reach filesize.
	if err == nil && lrs.offset >= lrs.size {
		err = io.EOF
	}

	// TODO(stevvooe): More error checking is required here. If the reader
	// times out for some reason, we should reset the reader so we re-open the
	// connection.

	return n, err
}

func (lrs *layerReader) Seek(offset int64, whence int) (int64, error) {
	if err := lrs.closed(); err != nil {
		return 0, err
	}

	var err error
	newOffset := lrs.offset

	switch whence {
	case os.SEEK_CUR:
		newOffset += int64(whence)
	case os.SEEK_END:
		newOffset = lrs.size + int64(whence)
	case os.SEEK_SET:
		newOffset = int64(whence)
	}

	if newOffset < 0 {
		err = fmt.Errorf("cannot seek to negative position")
	} else if newOffset >= lrs.size {
		err = fmt.Errorf("cannot seek passed end of layer")
	} else {
		if lrs.offset != newOffset {
			lrs.resetReader()
		}

		// No problems, set the offset.
		lrs.offset = newOffset
	}

	return lrs.offset, err
}

// Close the layer. Should be called when the resource is no longer needed.
func (lrs *layerReader) Close() error {
	if lrs.closedErr != nil {
		return lrs.closedErr
	}
	// TODO(sday): Must export this error.
	lrs.closedErr = fmt.Errorf("layer closed")

	// close and release reader chain
	if lrs.rc != nil {
		lrs.rc.Close()
		lrs.rc = nil
	}
	lrs.brd = nil

	return lrs.closedErr
}

// reader prepares the current reader at the lrs offset, ensuring its buffered
// and ready to go.
func (lrs *layerReader) reader() (io.Reader, error) {
	if err := lrs.closed(); err != nil {
		return nil, err
	}

	if lrs.rc != nil {
		return lrs.brd, nil
	}

	// If we don't have a reader, open one up.
	rc, err := lrs.layerStore.driver.ReadStream(lrs.path, uint64(lrs.offset))

	if err != nil {
		return nil, err
	}

	lrs.rc = rc

	if lrs.brd == nil {
		// TODO(stevvooe): Set an optimal buffer size here. We'll have to
		// understand the latency characteristics of the underlying network to
		// set this correctly, so we may want to leave it to the driver. For
		// out of process drivers, we'll have to optimize this buffer size for
		// local communication.
		lrs.brd = bufio.NewReader(lrs.rc)
	} else {
		lrs.brd.Reset(lrs.rc)
	}

	return lrs.brd, nil
}

// resetReader resets the reader, forcing the read method to open up a new
// connection and rebuild the buffered reader. This should be called when the
// offset and the reader will become out of sync, such as during a seek
// operation.
func (lrs *layerReader) resetReader() {
	if err := lrs.closed(); err != nil {
		return
	}
	if lrs.rc != nil {
		lrs.rc.Close()
		lrs.rc = nil
	}
}

func (lrs *layerReader) closed() error {
	return lrs.closedErr
}
