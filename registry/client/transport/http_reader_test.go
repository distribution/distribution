package transport

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
)

type trackedBody struct {
	*bytes.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

type stubRT struct{ resp *http.Response }

func (rt stubRT) RoundTrip(*http.Request) (*http.Response, error) { return rt.resp, nil }

func TestReaderClosesBodyOnRangeError(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		contentRange string
	}{
		{"wrong code for byte range", http.StatusOK, ""},
		{"missing content-range", http.StatusPartialContent, ""},
		{"unparsable content-range", http.StatusPartialContent, "bytes garbage"},
		{"wrong start offset", http.StatusPartialContent, "bytes 0-9/10"},
		{"stops before end", http.StatusPartialContent, "bytes 5-8/10"},
		{"error status", http.StatusInternalServerError, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: bytes.NewReader([]byte("hello"))}
			resp := &http.Response{
				StatusCode: tc.status,
				Status:     http.StatusText(tc.status),
				Header:     http.Header{},
				Body:       body,
			}
			if tc.contentRange != "" {
				resp.Header.Set("Content-Range", tc.contentRange)
			}

			hrs := &HTTPReadSeeker{
				client:       &http.Client{Transport: stubRT{resp}},
				url:          "http://example.com/blob",
				readerOffset: 5,
			}

			if _, err := hrs.reader(); err == nil {
				t.Fatalf("expected an error")
			}
			if !body.closed {
				t.Fatalf("response body was not closed")
			}
			if hrs.rc != nil {
				t.Fatalf("hrs.rc should stay nil on error")
			}
		})
	}
}

func TestReaderKeepsBodyOnSuccess(t *testing.T) {
	body := &trackedBody{Reader: bytes.NewReader([]byte("56789"))}
	resp := &http.Response{
		StatusCode: http.StatusPartialContent,
		Header:     http.Header{"Content-Range": []string{"bytes 5-9/10"}},
		Body:       body,
	}
	hrs := &HTTPReadSeeker{
		client:       &http.Client{Transport: stubRT{resp}},
		url:          "http://example.com/blob",
		readerOffset: 5,
	}
	r, err := hrs.reader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body.closed {
		t.Fatalf("body must stay open on success")
	}
	if hrs.size != 10 {
		t.Fatalf("size = %d, want 10", hrs.size)
	}
	got, _ := ioutil.ReadAll(r)
	if string(got) != "56789" {
		t.Fatalf("body = %q", got)
	}
}
