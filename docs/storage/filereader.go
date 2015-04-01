package storage

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// TODO(stevvooe): Set an optimal buffer size here. We'll have to
// understand the latency characteristics of the underlying network to
// set this correctly, so we may want to leave it to the driver. For
// out of process drivers, we'll have to optimize this buffer size for
// local communication.
const fileReaderBufferSize = 4 << 20

// remoteFileReader provides a read seeker interface to files stored in
// storagedriver. Used to implement part of layer interface and will be used
// to implement read side of LayerUpload.
type fileReader struct {
	driver storagedriver.StorageDriver

	// identifying fields
	path    string
	size    int64     // size is the total size, must be set.
	modtime time.Time // TODO(stevvooe): This is not needed anymore.

	// mutable fields
	rc     io.ReadCloser // remote read closer
	brd    *bufio.Reader // internal buffered io
	offset int64         // offset is the current read offset
	err    error         // terminal error, if set, reader is closed
}

// newFileReader initializes a file reader for the remote file. The read takes
// on the offset and size at the time the reader is created. If the underlying
// file changes, one must create a new fileReader.
func newFileReader(driver storagedriver.StorageDriver, path string) (*fileReader, error) {
	rd := &fileReader{
		driver: driver,
		path:   path,
	}

	// Grab the size of the layer file, ensuring existence.
	if fi, err := driver.Stat(path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// NOTE(stevvooe): We really don't care if the file is not
			// actually present for the reader. If the caller needs to know
			// whether or not the file exists, they should issue a stat call
			// on the path. There is still no guarantee, since the file may be
			// gone by the time the reader is created. The only correct
			// behavior is to return a reader that immediately returns EOF.
		default:
			// Any other error we want propagated up the stack.
			return nil, err
		}
	} else {
		if fi.IsDir() {
			return nil, fmt.Errorf("cannot read a directory")
		}

		// Fill in file information
		rd.size = fi.Size()
		rd.modtime = fi.ModTime()
	}

	return rd, nil
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
	} else {
		if fr.offset != newOffset {
			fr.reset()
		}

		// No problems, set the offset.
		fr.offset = newOffset
	}

	return fr.offset, err
}

func (fr *fileReader) Close() error {
	return fr.closeWithErr(fmt.Errorf("fileReader: closed"))
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
	rc, err := fr.driver.ReadStream(fr.path, fr.offset)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// NOTE(stevvooe): If the path is not found, we simply return a
			// reader that returns io.EOF. However, we do not set fr.rc,
			// allowing future attempts at getting a reader to possibly
			// succeed if the file turns up later.
			return ioutil.NopCloser(bytes.NewReader([]byte{})), nil
		default:
			return nil, err
		}
	}

	fr.rc = rc

	if fr.brd == nil {
		// TODO(stevvooe): Set an optimal buffer size here. We'll have to
		// understand the latency characteristics of the underlying network to
		// set this correctly, so we may want to leave it to the driver. For
		// out of process drivers, we'll have to optimize this buffer size for
		// local communication.
		fr.brd = bufio.NewReaderSize(fr.rc, fileReaderBufferSize)
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

func (fr *fileReader) closeWithErr(err error) error {
	if fr.err != nil {
		return fr.err
	}

	fr.err = err

	// close and release reader chain
	if fr.rc != nil {
		fr.rc.Close()
	}

	fr.rc = nil
	fr.brd = nil

	return fr.err
}
