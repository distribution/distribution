package distribution

import (
	"bytes"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/docker/distribution/digest"
)

func TestReadSeekCloser(t *testing.T) {
	dgst, p := randomBytes(t, 1024)
	br := NewReadSeekCloser(int64(len(p)), bytes.NewReader(p))

	// read tests
	allRead, err := ioutil.ReadAll(br)
	if err != nil {
		t.Fatalf("unexpected error reading all of blob: %v", err)
	}

	if !reflect.DeepEqual(allRead, p) {
		t.Fatalf("data read does not match: %v", err)
	}

	// Double check the digest.
	allReadDgst, err := digest.FromBytes(allRead)
	if err != nil {
		t.Fatalf("unexpected error digesting all: %v", err)
	}

	if allReadDgst != dgst {
		t.Fatalf("unexpected digest from all: %q != %q", allReadDgst, dgst)
	}

	// check eof
	var buf [1024]byte
	n, err := br.Read(buf[:])
	if err != io.EOF {
		t.Fatalf("unexpected error or nil when reading from end of blob reader: %v", err)
	}

	if n != 0 {
		t.Fatalf("read more than zero bytes: %v != %v", n, 0)
	}

	// close tests
	if err := br.Close(); err != nil {
		t.Fatalf("unexpected error closing reader: %v", err)
	}

	if err := br.Close(); err == nil {
		t.Fatalf("expected error on double close: %v", err)
	}
}

func TestReadSeekCloserSeeking(t *testing.T) {
	// NOTE(stevvooe): This seek test is nearly verbatim copied for the
	// fileReader tests in the registry/storage package. This implementation
	// should replace it eventually. There is an argument for adding
	// "standardized" seek tests but it may be not be worth it.

	pattern := "01234567890ab" // prime length block
	repititions := 1024
	content := bytes.Repeat([]byte(pattern), repititions)
	blob, err := NewBlobFromBytes("application/octet-stream", content)
	if err != nil {
		t.Fatalf("unexpected error creating byte blob: %v", err)
	}

	br, err := blob.Reader()
	if err != nil {
		t.Fatalf("unexpected error getting reader: %v", err)
	}
	defer br.Close()

	// Seek all over the place, in blocks of pattern size and make sure we get
	// the right data.
	for _, repitition := range mrand.Perm(repititions - 1) {
		targetOffset := int64(len(pattern) * repitition)
		// Seek to a multiple of pattern size and read pattern size bytes
		offset, err := br.Seek(targetOffset, os.SEEK_SET)
		if err != nil {
			t.Fatalf("unexpected error seeking: %v", err)
		}

		if offset != targetOffset {
			t.Fatalf("did not seek to correct offset: %d != %d", offset, targetOffset)
		}

		p := make([]byte, len(pattern))

		n, err := br.Read(p)
		if err != nil {
			t.Fatalf("error reading pattern: %v", err)
		}

		if n != len(pattern) {
			t.Fatalf("incorrect read length: %d != %d", n, len(pattern))
		}

		if string(p) != pattern {
			t.Fatalf("incorrect read content: %q != %q", p, pattern)
		}

		// Check offset
		current, err := br.Seek(0, os.SEEK_CUR)
		if err != nil {
			t.Fatalf("error checking current offset: %v", err)
		}

		if current != targetOffset+int64(len(pattern)) {
			t.Fatalf("unexpected offset after read: %v", err)
		}
	}

	start, err := br.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatalf("error seeking to start: %v", err)
	}

	if start != 0 {
		t.Fatalf("expected to seek to start: %v != 0", start)
	}

	end, err := br.Seek(0, os.SEEK_END)
	if err != nil {
		t.Fatalf("error checking current offset: %v", err)
	}

	if end != int64(len(content)) {
		t.Fatalf("expected to seek to end: %v != %v", end, len(content))
	}

	// seek before start
	before, err := br.Seek(-1, os.SEEK_SET)
	if err == nil {
		t.Fatalf("error expected, returned offset=%v", before)
	}

	// seek after end,
	after, err := br.Seek(1, os.SEEK_END)
	if err != nil {
		t.Fatalf("unexpected error expected, returned offset=%v", after)
	}

	if after != int64(len(content))+1 {
		t.Fatalf("unexpected seek offset: %v != %v", after, int64(len(content))+1)
	}

	p := make([]byte, 16)
	n, err := br.Read(p)

	if n != 0 {
		t.Fatalf("bytes reads %d != %d", n, 0)
	}

	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}
