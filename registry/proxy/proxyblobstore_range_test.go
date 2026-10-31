package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/notifications"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Exercise the actual origin client and configuration wiring, including the
// notification decorator. HEAD requests for metadata are not blob downloads.
func TestProxyRangePassthrough(t *testing.T) {
	data := []byte("0123456789abcdef")
	dgst := digest.FromBytes(data)
	for _, tc := range []struct {
		name, setting, env string
		enabled            bool
	}{
		{name: "omitted"},
		{name: "disabled", setting: "rangepassthrough: false"},
		{name: "enabled", setting: "rangepassthrough: true", enabled: true},
		{name: "environment enabled", env: "true", enabled: true},
		{name: "environment disabled", setting: "rangepassthrough: true", env: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REGISTRY_PROXY_RANGEPASSTHROUGH", tc.env)
			if tc.env == "" {
				if err := os.Unsetenv("REGISTRY_PROXY_RANGEPASSTHROUGH"); err != nil {
					t.Fatal(err)
				}
			}
			var ranges []string
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v2/" {
					w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				user, password, ok := r.BasicAuth()
				if !ok || user != "origin-user" || password != "origin-password" {
					http.Error(w, "missing origin credentials", http.StatusUnauthorized)
					return
				}
				w.Header().Set("Docker-Content-Digest", dgst.String())
				w.Header().Set("Content-Type", "application/octet-stream")
				if r.Method == http.MethodHead {
					w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					return
				}
				ranges = append(ranges, r.Header.Get("Range"))
				http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
			}))
			defer origin.Close()
			config, err := configuration.Parse(strings.NewReader(fmt.Sprintf(`version: 0.1
storage:
  inmemory: {}
proxy:
  remoteurl: %s
  username: origin-user
  password: origin-password
  ttl: 0
  %s
`, origin.URL, tc.setting)))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			driver := inmemory.New()
			local, err := storage.NewRegistry(ctx, driver)
			if err != nil {
				t.Fatal(err)
			}
			namespace, err := NewRegistryPullThroughCache(ctx, local, driver, config.Proxy)
			if err != nil {
				t.Fatal(err)
			}
			defer namespace.(Closer).Close()
			name, _ := reference.WithName("test/blob")
			repo, err := namespace.Repository(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			pbs := repo.Blobs(ctx).(*proxyBlobStore)
			stats := statsBlobStore{stats: make(map[string]int), blobs: pbs.localStore}
			pbs.localStore = stats
			listener := &rangePullListener{}
			repo, _ = notifications.Listen(repo, nil, listener)
			blobs := repo.Blobs(ctx)
			serve := func(value, ifRange string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Range", value)
				req.Header.Set("If-Range", ifRange)
				req.Header.Set("Authorization", "Bearer downstream-only")
				w := httptest.NewRecorder()
				if err := blobs.ServeBlob(ctx, w, req, dgst); err != nil {
					t.Fatal(err)
				}
				return w
			}
			before := proxyMetrics.blobMetrics
			w := serve("bytes=2-5", "")
			wantStatus, wantBody, wantRange := http.StatusOK, string(data), ""
			if tc.enabled {
				wantStatus, wantBody, wantRange = http.StatusPartialContent, "2345", "bytes=2-5"
			}
			if w.Code != wantStatus || w.Body.String() != wantBody || len(ranges) != 1 || ranges[0] != wantRange {
				t.Fatalf("response %d %q, origin ranges %v", w.Code, w.Body.String(), ranges)
			}
			if listener.pulls != 1 || listener.size != int64(len(data)) {
				t.Fatalf("pull descriptor: %+v", listener)
			}
			if tc.enabled {
				if w.Header().Get("Content-Range") != "bytes 2-5/16" || w.Header().Get("Content-Length") != "4" {
					t.Fatal(w.Header())
				}
				if _, err := stats.Stat(ctx, dgst); err != distribution.ErrBlobUnknown || stats.stats["create"] != 0 {
					t.Fatalf("range populated cache: %v %v", err, stats.stats)
				}
				after := proxyMetrics.blobMetrics
				if after.BytesPulled-before.BytesPulled != 4 || after.BytesPushed-before.BytesPushed != 4 {
					t.Fatalf("range metrics: before=%+v after=%+v", before, after)
				}
				w = serve("bytes=99-", "")
				if w.Code != http.StatusRequestedRangeNotSatisfiable || listener.pulls != 1 || proxyMetrics.blobMetrics != after {
					t.Fatalf("416 counted as a pull: status=%d pulls=%d metrics=%+v", w.Code, listener.pulls, proxyMetrics.blobMetrics)
				}
				w = serve("bytes=2-5", `"stale"`)
				if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) || stats.stats["create"] != 0 {
					t.Fatal("origin 200 fallback was changed or cached")
				}
				w = serve("", "")
				if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) {
					t.Fatal("ordinary GET changed")
				}
			}
			if stats.stats["create"] != 1 {
				t.Fatalf("cache writers: %v", stats.stats)
			}
			if _, err := stats.Stat(ctx, dgst); err != nil {
				t.Fatal(err)
			}
			originGets := len(ranges)
			w = serve("bytes=6-9", "")
			if w.Code != http.StatusPartialContent || w.Body.String() != "6789" || len(ranges) != originGets {
				t.Fatalf("warm range: %d %q, origin ranges %v", w.Code, w.Body.String(), ranges)
			}
		})
	}
}

type rangePullListener struct {
	notifications.Listener
	pulls int
	size  int64
}

func (l *rangePullListener) BlobPulled(_ reference.Named, desc v1.Descriptor) error {
	l.pulls++
	l.size = desc.Size
	return nil
}
