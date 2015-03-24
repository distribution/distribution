package sha512

import (
	"bytes"
	stdlib "crypto"
	"crypto/rand"
	_ "crypto/sha512" // To register the stdlib sha224 and sha256 algs.
	resumable "github.com/jlhawn/go-crypto"
	"io"
	"testing"
)

func compareResumableHash(t *testing.T, r resumable.Hash, h stdlib.Hash) {
	// Read 3 Kilobytes of random data into a buffer.
	buf := make([]byte, 3*1024)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		t.Fatalf("unable to load random data: %s", err)
	}

	// Use two Hash objects to consume prefixes of the data. One will be
	// snapshotted and resumed with each additional byte, then both will write
	// that byte. The digests should be equal after each byte is digested.
	resumableHasher := r.New()
	stdlibHasher := h.New()

	// First, assert that the initial distest is the same.
	if !bytes.Equal(resumableHasher.Sum(nil), stdlibHasher.Sum(nil)) {
		t.Fatalf("initial digests do not match: got %x, expected %x", resumableHasher.Sum(nil), stdlibHasher.Sum(nil))
	}

	multiWriter := io.MultiWriter(resumableHasher, stdlibHasher)

	for i := 1; i <= len(buf); i++ {

		// Write the next byte.
		multiWriter.Write(buf[i-1 : i])

		if !bytes.Equal(resumableHasher.Sum(nil), stdlibHasher.Sum(nil)) {
			t.Fatalf("digests do not match: got %x, expected %x", resumableHasher.Sum(nil), stdlibHasher.Sum(nil))
		}

		// Snapshot, reset, and restore the chunk hasher.
		hashState, err := resumableHasher.State()
		if err != nil {
			t.Fatalf("unable to get state of hash function: %s", err)
		}
		resumableHasher.Reset()
		if err := resumableHasher.Restore(hashState); err != nil {
			t.Fatalf("unable to restorte state of hash function: %s", err)
		}
	}
}

func TestResumable(t *testing.T) {
	compareResumableHash(t, resumable.SHA384, stdlib.SHA384)
	compareResumableHash(t, resumable.SHA512, stdlib.SHA512)
}
