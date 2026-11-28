package registry

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/reference"
	"github.com/prometheus/client_golang/prometheus"
)

type fileObservingListener struct {
	net.Listener
	sawFile *atomic.Bool
}

func (l fileObservingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return fileObservingConn{c.(*net.TCPConn), l.sawFile}, nil
}

// fileObservingConn records whether net/http passes a file to TCPConn.ReadFrom.
type fileObservingConn struct {
	*net.TCPConn
	sawFile *atomic.Bool
}

func (c fileObservingConn) ReadFrom(r io.Reader) (int64, error) {
	if limited, ok := r.(*io.LimitedReader); ok {
		if _, ok := limited.R.(*os.File); ok {
			c.sawFile.Store(true)
		}
	}
	return c.TCPConn.ReadFrom(r)
}

// A filesystem blob GET must preserve the file through every response writer
// that the registry installs, including the optional Prometheus middleware.
func TestFilesystemBlobGETReachesTCPConnReadFrom(t *testing.T) {
	// Route registration installs metrics in the default Prometheus registry.
	previous := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	defer func() { prometheus.DefaultRegisterer = previous }()

	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	config := &configuration.Configuration{
		Storage: configuration.Storage{"filesystem": {"rootdirectory": t.TempDir()}},
	}
	config.HTTP.Debug.Prometheus.Enabled = true
	reg, err := NewRegistry(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.app.Shutdown()

	var sawFile atomic.Bool
	server := httptest.NewUnstartedServer(reg.server.Handler)
	server.Listener = fileObservingListener{server.Listener, &sawFile}
	server.Start()
	defer server.Close()

	name, _ := reference.WithName("test")
	repo, err := client.NewRepository(name, server.URL, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	// Leave data for ReadFrom after net/http's initial 512-byte copy.
	content := bytes.Repeat([]byte("0123456789"), 1000)
	desc, err := repo.Blobs(t.Context()).Put(t.Context(), "application/octet-stream", content)
	if err != nil {
		t.Fatal(err)
	}

	sawFile.Store(false)
	response, err := server.Client().Get(server.URL + "/v2/" + name.Name() + "/blobs/" + desc.Digest.String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("blob GET returned %s", response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, content) {
		t.Fatal("blob GET body differs from uploaded content")
	}
	if !sawFile.Load() {
		t.Error("blob file did not reach net.TCPConn.ReadFrom")
	}
}
