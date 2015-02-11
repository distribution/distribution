package storage

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"testing"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver/inmemory"
)

// TestSimpleWrite takes the fileWriter through common write operations
// ensuring data integrity.
func TestSimpleWrite(t *testing.T) {
	content := make([]byte, 1<<20)
	n, err := rand.Read(content)
	if err != nil {
		t.Fatalf("unexpected error building random data: %v", err)
	}

	if n != len(content) {
		t.Fatalf("random read did't fill buffer")
	}

	dgst, err := digest.FromReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error digesting random content: %v", err)
	}

	driver := inmemory.New()
	path := "/random"

	fw, err := newFileWriter(driver, path)
	if err != nil {
		t.Fatalf("unexpected error creating fileWriter: %v", err)
	}
	defer fw.Close()

	n, err = fw.Write(content)
	if err != nil {
		t.Fatalf("unexpected error writing content: %v", err)
	}

	if n != len(content) {
		t.Fatalf("unexpected write length: %d != %d", n, len(content))
	}

	fr, err := newFileReader(driver, path)
	if err != nil {
		t.Fatalf("unexpected error creating fileReader: %v", err)
	}
	defer fr.Close()

	verifier := digest.NewDigestVerifier(dgst)
	io.Copy(verifier, fr)

	if !verifier.Verified() {
		t.Fatalf("unable to verify write data")
	}

	// Check the seek position is equal to the content length
	end, err := fw.Seek(0, os.SEEK_END)
	if err != nil {
		t.Fatalf("unexpected error seeking: %v", err)
	}

	if end != int64(len(content)) {
		t.Fatalf("write did not advance offset: %d != %d", end, len(content))
	}

	// Double the content, but use the WriteAt method
	doubled := append(content, content...)
	doubledgst, err := digest.FromReader(bytes.NewReader(doubled))
	if err != nil {
		t.Fatalf("unexpected error digesting doubled content: %v", err)
	}

	n, err = fw.WriteAt(content, end)
	if err != nil {
		t.Fatalf("unexpected error writing content at %d: %v", end, err)
	}

	if n != len(content) {
		t.Fatalf("writeat was short: %d != %d", n, len(content))
	}

	fr, err = newFileReader(driver, path)
	if err != nil {
		t.Fatalf("unexpected error creating fileReader: %v", err)
	}
	defer fr.Close()

	verifier = digest.NewDigestVerifier(doubledgst)
	io.Copy(verifier, fr)

	if !verifier.Verified() {
		t.Fatalf("unable to verify write data")
	}

	// Check that WriteAt didn't update the offset.
	end, err = fw.Seek(0, os.SEEK_END)
	if err != nil {
		t.Fatalf("unexpected error seeking: %v", err)
	}

	if end != int64(len(content)) {
		t.Fatalf("write did not advance offset: %d != %d", end, len(content))
	}

	// Now, we copy from one path to another, running the data through the
	// fileReader to fileWriter, rather than the driver.Move command to ensure
	// everything is working correctly.
	fr, err = newFileReader(driver, path)
	if err != nil {
		t.Fatalf("unexpected error creating fileReader: %v", err)
	}
	defer fr.Close()

	fw, err = newFileWriter(driver, "/copied")
	if err != nil {
		t.Fatalf("unexpected error creating fileWriter: %v", err)
	}
	defer fw.Close()

	nn, err := io.Copy(fw, fr)
	if err != nil {
		t.Fatalf("unexpected error copying data: %v", err)
	}

	if nn != int64(len(doubled)) {
		t.Fatalf("unexpected copy length: %d != %d", nn, len(doubled))
	}

	fr, err = newFileReader(driver, "/copied")
	if err != nil {
		t.Fatalf("unexpected error creating fileReader: %v", err)
	}
	defer fr.Close()

	verifier = digest.NewDigestVerifier(doubledgst)
	io.Copy(verifier, fr)

	if !verifier.Verified() {
		t.Fatalf("unable to verify write data")
	}
}
