package transport

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestContentEncoding(t *testing.T) {
	t.Parallel()

	zstdDecode := func(in []byte) []byte {
		var b bytes.Buffer
		zw, err := zstd.NewWriter(&b)
		if err != nil {
			t.Fatal(err)
		}
		_, err = zw.Write(in)
		if err != nil {
			t.Fatal()
		}
		err = zw.Close()
		if err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	gzipEncode := func(in []byte) []byte {
		var b bytes.Buffer
		gw := gzip.NewWriter(&b)
		_, err := gw.Write(in)
		if err != nil {
			t.Fatal(err)
		}
		err = gw.Close()
		if err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	flateEncode := func(in []byte) []byte {
		var b bytes.Buffer
		dw, err := flate.NewWriter(&b, -1)
		if err != nil {
			t.Fatal(err)
		}
		_, err = dw.Write(in)
		if err != nil {
			t.Fatal(err)
		}
		err = dw.Close()
		if err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}

	tests := []struct {
		encodingFuncs  []func([]byte) []byte
		encodingHeader string
	}{
		{
			encodingFuncs:  []func([]byte) []byte{},
			encodingHeader: "",
		},
		{
			encodingFuncs:  []func([]byte) []byte{zstdDecode},
			encodingHeader: "zstd",
		},
		{
			encodingFuncs:  []func([]byte) []byte{gzipEncode},
			encodingHeader: "gzip",
		},
		{
			encodingFuncs:  []func([]byte) []byte{flateEncode},
			encodingHeader: "deflate",
		},
		{
			encodingFuncs:  []func([]byte) []byte{zstdDecode, gzipEncode},
			encodingHeader: "zstd,gzip",
		},
		{
			encodingFuncs:  []func([]byte) []byte{gzipEncode, flateEncode},
			encodingHeader: "gzip,deflate",
		},
		{
			encodingFuncs:  []func([]byte) []byte{gzipEncode, zstdDecode},
			encodingHeader: "gzip,zstd",
		},
		{
			encodingFuncs:  []func([]byte) []byte{gzipEncode, zstdDecode, flateEncode},
			encodingHeader: "gzip,zstd,deflate",
		},
	}

	for _, tc := range tests {
		t.Run(tc.encodingHeader, func(t *testing.T) {
			t.Parallel()
			content := make([]byte, 128)
			rand.New(rand.NewSource(1)).Read(content)

			s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				compressedContent := content
				for _, enc := range tc.encodingFuncs {
					compressedContent = enc(compressedContent)
				}
				rw.Header().Set("content-length", fmt.Sprintf("%d", len(compressedContent)))
				rw.Header().Set("Content-Encoding", tc.encodingHeader)
				_, _ = rw.Write(compressedContent)
			}))
			defer s.Close()

			u, err := url.Parse(s.URL)
			if err != nil {
				t.Fatal(err)
			}

			rs := NewHTTPReadSeeker(context.TODO(), http.DefaultClient, u.String(), func(r *http.Response) error { return nil })

			b, err := io.ReadAll(rs)
			if err != nil {
				t.Fatal(err)
			}
			expected := content
			if len(b) != len(expected) {
				t.Errorf("unexpected length %d, expected %d", len(b), len(expected))
				return
			}
			for i, c := range expected {
				if b[i] != c {
					t.Errorf("unexpected byte %x at %d, expected %x", b[i], i, c)
					return
				}
			}
		})
	}
}

func TestSeekWithUnknownTotalSize(t *testing.T) {
	t.Parallel()

	content := make([]byte, 128)
	rand.New(rand.NewSource(1)).Read(content)

	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Respond to a range request with a 206 whose Content-Range
		// reports an unknown complete length ("*"), as permitted by
		// RFC 7233.
		var start int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &start); err != nil || start < 0 || start > len(content) {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", start, len(content)-1))
		rw.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)-start))
		rw.WriteHeader(http.StatusPartialContent)
		_, _ = rw.Write(content[start:])
	}))
	defer s.Close()

	rs := NewHTTPReadSeeker(t.Context(), http.DefaultClient, s.URL, func(r *http.Response) error { return nil })
	defer rs.Close()

	const offset = 64
	if _, err := rs.Seek(offset, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	b, err := io.ReadAll(rs)
	if err != nil {
		t.Fatal(err)
	}

	expected := content[offset:]
	if !bytes.Equal(b, expected) {
		t.Errorf("unexpected content: got %d bytes, expected %d", len(b), len(expected))
	}
}
