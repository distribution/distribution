package distribution

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/docker/distribution/digest"
)

// bytes is a blob implementation based on byte-slice. Not all blobs can be
// represented as bytes arrays if they're length cannot be represented as an
// int.
type bytesBlob struct {
	desc Descriptor
	rd   bytes.Reader
	err  error // set when closed
}

// NewBlob reads the rd and creates a blob from the data, calculating the size
// and digest. The resulting descriptor will be filled with values from rd and
// the passed mediaType. Note that this blob will only exist in memory and the
// caller will have to take action to write it into an existing service. Avoid
// using this for very large blobs.
func NewBlob(mediaType string, rd io.Reader) (Blob, error) {
	digester := digest.NewCanonicalDigester()
	rd = io.TeeReader(rd, &digester)

	p, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("new blob: %v", err)
	}

	return &bytesBlob{
		desc: Descriptor{
			Length:    int64(len(p)),
			MediaType: mediaType,
			Digest:    digester.Digest(),
		},
		rd: *bytes.NewReader(p), // copy in, since we will clone it.
	}, nil
}

// NewBlobFromBytes returns a blob from the byte slice with the provided
// mediaType. Instance is not associated with a service but fulfulls the Blob
// interface.
func NewBlobFromBytes(mediaType string, p []byte) (Blob, error) {
	return NewBlob(mediaType, bytes.NewReader(p))
}

func (b *bytesBlob) Descriptor() Descriptor {
	return b.desc
}

func (b *bytesBlob) Reader() (ReadSeekCloser, error) {
	if b.err != nil {
		return nil, b.err
	}

	// NOTE(stevvooe): Most implementations should use NewReadSeekCloser.
	// However, we can clone out a bytes.Reader since we have an actual byte
	// slice.
	cbr := &closingByteReader{
		rd: b.rd, // takes a clone
	}

	return cbr, cbr.reset() // return reset error, if it happens.
}

func (b *bytesBlob) ReadAt(p []byte, off int64) (n int, err error) {
	return b.rd.ReadAt(p, off)
}

func (b *bytesBlob) Close() error {
	if b.err != nil {
		return b.err
	}

	b.err = fmt.Errorf("bytesBlob: closed")
	return nil
}

func (b *bytesBlob) Handler(r *http.Request) (http.Handler, error) {
	if b.err != nil {
		return nil, b.err
	}
	// TODO(stevvooe): This can be factored into a more widely useful method,
	// such as ServeBlob which other implementations can use to directly serve
	// blob data.

	rd, err := b.Reader()
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer rd.Close()

		// TODO(stevvooe): Need to set a few headers here, such as Docker-
		// Content-Digest and maybe some etags using the digest.
		http.ServeContent(w, r, b.Descriptor().Digest.String(), time.Time{}, rd)
	}), nil
}

// closingByteReader is just a bytes.Reader with a close method.
type closingByteReader struct {
	rd  bytes.Reader
	err error
}

func (cbr *closingByteReader) Read(p []byte) (n int, err error) {
	if cbr.err != nil {
		return 0, cbr.err
	}
	return cbr.rd.Read(p)
}

func (cbr *closingByteReader) Seek(offset int64, whence int) (int64, error) {
	if cbr.err != nil {
		return 0, cbr.err
	}
	return cbr.rd.Seek(offset, whence)
}

func (cbr *closingByteReader) Close() error {
	if cbr.err != nil {
		return cbr.err
	}

	cbr.err = fmt.Errorf("closingByteReader: closed")
	return nil
}

func (cbr *closingByteReader) reset() error {
	// reset our local reader by seeking to start (paranoid)
	off, err := cbr.rd.Seek(0, os.SEEK_SET)
	if err != nil {
		return err
	}

	// seriously paranoid
	if off != 0 {
		return fmt.Errorf("closingByteReader: unable to seek to start")
	}

	return nil
}
