package distribution

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/docker/distribution/digest"
)

func TestBytesBlob(t *testing.T) {
	const blobLength = 1024
	const mediaType = "application/foo"
	blob, dgst, p := randomBlob(t, mediaType, blobLength)

	if blob.Descriptor().Length != blobLength {
		t.Fatalf("unexpected blob length: %v != %v", blob.Descriptor().Length, blobLength)
	}

	if blob.Descriptor().Digest != dgst {
		t.Fatalf("unexpected digest: %q != %q", blob.Descriptor().Digest, dgst)
	}

	if blob.Descriptor().MediaType != mediaType {
		t.Fatalf("unexpected digest: %q != %q", blob.Descriptor().Digest, dgst)
	}

	actual := make([]byte, blobLength)
	n, err := blob.ReadAt(actual, 0)
	if err != nil {
		t.Fatalf("unexpected error readat:", err)
	}

	if n != blobLength {
		t.Fatalf("full read didn't happed: %v != %v", n, blobLength)
	}

	if !bytes.Equal(actual, p) {
		t.Fatalf("incorrect data return from ReadAt, bytes not equal")
	}

	// read at end and get eof
	n, err = blob.ReadAt(actual, blobLength)
	if err != io.EOF {
		t.Fatalf("EOF not received when reading end of blob: %v", err)
	}

	if n != 0 {
		t.Fatalf("should have read no bytes: %v != %v", n, 0)
	}

	// read before end, get one byte.
	n, err = blob.ReadAt(actual[:1], blobLength-1)
	if err == io.EOF {
		t.Fatalf("EOF received when reading before end of blob: %v", err)
	}

	if n != 1 {
		t.Fatalf("should have read 1 bytes: %v != %v", n, 1)
	}

	if err := blob.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}

	// now, we should get an error on close or any other method.
	if err := blob.Close(); err == nil {
		t.Fatalf("second close did not yield error")
	}

	if _, err := blob.Handler(nil); err == nil {
		t.Fatalf("second close did not yield error other method")
	}
}

// randomBlob creates a random blob with media type and returns a blob and
// byte buffer with the raw content.
func randomBlob(t *testing.T, mediaType string, length int) (Blob, digest.Digest, []byte) {
	dgst, p := randomBytes(t, length)

	blob, err := NewBlobFromBytes(mediaType, p)
	if err != nil {
		t.Fatalf("error creating blob: %v", err)
	}

	return blob, dgst, p
}

func randomBytes(t *testing.T, length int) (digest.Digest, []byte) {
	p := make([]byte, length)
	n, err := io.ReadFull(rand.Reader, p)
	if err != nil {
		t.Fatalf("unexpected error reading rand: %v", err)
	}

	if n != length {
		t.Fatalf("read unexpected number of bytes: %v != %d", n, length)
	}

	dgst, err := digest.FromBytes(p)
	if err != nil {
		t.Fatalf("error digesting bytes: %v", err)
	}

	return dgst, p
}
