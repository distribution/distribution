package registry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/handlers"
	"github.com/opencontainers/go-digest"
)

// Include the dispatcher and outer recovery middleware: a truncated origin
// stream must not become a completed response or acquire a JSON error suffix.
func TestProxyRangeStreamAbort(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 2048)
	dgst := digest.FromBytes(data)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Docker-Content-Digest", dgst.String())
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			return
		}
		if r.Header.Get("Range") != "bytes=4096-20479" {
			t.Errorf("origin Range: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 4096-20479/32768")
		w.Header().Set("Content-Length", "16384")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[4096:12288])
		w.(http.Flusher).Flush()
	}))
	defer origin.Close()
	config := &configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory":    configuration.Parameters{},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{"enabled": false}},
		},
		Proxy: configuration.Proxy{RemoteURL: origin.URL, RangePassthrough: true, TTL: new(time.Duration)},
	}
	app := handlers.NewApp(context.Background(), config)
	defer app.Shutdown()
	aborted := make(chan any, 1)
	handler := panicHandler(app)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				aborted <- p
				panic(p)
			}
		}()
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v2/test/blob/blobs/"+dgst.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=4096-20479")
	client := server.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || !errors.Is(err, io.ErrUnexpectedEOF) || len(body) == 0 || len(body) >= 16384 || !bytes.Equal(body, data[4096:4096+len(body)]) {
		t.Fatalf("stream was not aborted cleanly: status=%d bytes=%d err=%v", resp.StatusCode, len(body), err)
	}
	select {
	case p := <-aborted:
		if p != http.ErrAbortHandler {
			t.Fatalf("recovery changed abort sentinel: %v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not abort")
	}
}
