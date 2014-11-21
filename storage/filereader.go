package storage

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker-registry/storagedriver"
)

// remoteFileReader provides a read seeker interface to files stored in
// storagedriver. Used to implement part of layer interface and will be used
// to implement read side of LayerUpload.
type fileReader struct {
	driver storagedriver.StorageDriver

	// identifying fields
	path string
	size int64 // size is the total layer size, must be set.

	// mutable fields
	rc     io.ReadCloser // remote read closer
	brd    *bufio.Reader // internal buffered io
	offset int64         // offset is the current read offset
	err    error         // terminal error, if set, reader is closed
}

func newFileReader(driver storagedriver.StorageDriver, path string) (*fileReader, error) {
	// Grab the size of the layer file, ensuring existence.
	size, err := driver.CurrentSize(path)

	if err != nil {
		return nil, err
	}

	return &fileReader{
		driver: driver,
		path:   path,
		size:   int64(size),
	}, nil
}

func (fr *fileReader) Read(p []byte) (n int, err error) {
	if fr.err != nil {
		return 0, fr.err
	}

	rd, err := fr.reader()
	if err != nil {
		return 0, err
	}

	n, err = rd.Read(p)
	fr.offset += int64(n)

	// Simulate io.EOR error if we reach filesize.
	if err == nil && fr.offset >= fr.size {
		err = io.EOF
	}

	return n, err
}

func (fr *fileReader) Seek(offset int64, whence int) (int64, error) {
	if fr.err != nil {
		return 0, fr.err
	}

	var err error
	newOffset := fr.offset

	switch whence {
	case os.SEEK_CUR:
		newOffset += int64(offset)
	case os.SEEK_END:
		newOffset = fr.size + int64(offset)
	case os.SEEK_SET:
		newOffset = int64(offset)
	}

	if newOffset < 0 {
		err = fmt.Errorf("cannot seek to negative position")
	} else if newOffset > fr.size {
		err = fmt.Errorf("cannot seek passed end of file")
	} else {
		if fr.offset != newOffset {
			fr.reset()
		}

		// No problems, set the offset.
		fr.offset = newOffset
	}

	return fr.offset, err
}

// Close the layer. Should be called when the resource is no longer needed.
func (fr *fileReader) Close() error {
	if fr.err != nil {
		return fr.err
	}

	fr.err = ErrLayerClosed

	// close and release reader chain
	if fr.rc != nil {
		fr.rc.Close()
	}

	fr.rc = nil
	fr.brd = nil

	return fr.err
}

// reader prepares the current reader at the lrs offset, ensuring its buffered
// and ready to go.
func (fr *fileReader) reader() (io.Reader, error) {
	if fr.err != nil {
		return nil, fr.err
	}

	if fr.rc != nil {
		return fr.brd, nil
	}

	// If we don't have a reader, open one up.
	rc, err := fr.driver.ReadStream(fr.path, uint64(fr.offset))

	if err != nil {
		return nil, err
	}

	fr.rc = rc

	if fr.brd == nil {
		// TODO(stevvooe): Set an optimal buffer size here. We'll have to
		// understand the latency characteristics of the underlying network to
		// set this correctly, so we may want to leave it to the driver. For
		// out of process drivers, we'll have to optimize this buffer size for
		// local communication.
		fr.brd = bufio.NewReader(fr.rc)
	} else {
		fr.brd.Reset(fr.rc)
	}

	return fr.brd, nil
}

// resetReader resets the reader, forcing the read method to open up a new
// connection and rebuild the buffered reader. This should be called when the
// offset and the reader will become out of sync, such as during a seek
// operation.
func (fr *fileReader) reset() {
	if fr.err != nil {
		return
	}
	if fr.rc != nil {
		fr.rc.Close()
		fr.rc = nil
	}
}
