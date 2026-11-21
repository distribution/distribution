package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

func TestProxyingRegistryCloseWithoutScheduler(t *testing.T) {
	pr := &proxyingRegistry{
		scheduler: nil,
	}

	// verify that `Close()` does not panic when the scheduler is nil
	err := pr.Close()
	if err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
}

func TestNewUpstreamTransportDoesNotModifyDefaultTransport(t *testing.T) {
	rt := newUpstreamTransport(upstreamSendPingTimeout, upstreamPingTimeout)
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("newUpstreamTransport() returned %T, want *http.Transport", rt)
	}
	if tr == http.DefaultTransport {
		t.Fatal("newUpstreamTransport() returned http.DefaultTransport itself")
	}
	if tr.HTTP2 == nil || tr.HTTP2.SendPingTimeout != upstreamSendPingTimeout || tr.HTTP2.PingTimeout != upstreamPingTimeout {
		t.Fatalf("unexpected HTTP2 config: %+v", tr.HTTP2)
	}
	if h2 := http.DefaultTransport.(*http.Transport).HTTP2; h2 != nil && (h2.SendPingTimeout != 0 || h2.PingTimeout != 0) {
		t.Fatalf("http.DefaultTransport was modified: %+v", h2)
	}
}

func TestNewRegistryPullThroughCacheUsesUpstreamTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(upstream.Close)

	ctx := context.Background()
	driver := inmemory.New()
	local, err := storage.NewRegistry(ctx, driver)
	if err != nil {
		t.Fatal(err)
	}
	ttl := time.Duration(0)
	ns, err := NewRegistryPullThroughCache(ctx, local, driver, configuration.Proxy{RemoteURL: upstream.URL, TTL: &ttl})
	if err != nil {
		t.Fatal(err)
	}

	pr := ns.(*proxyingRegistry)
	tr, ok := pr.transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", pr.transport)
	}
	if tr.HTTP2 == nil || tr.HTTP2.SendPingTimeout != upstreamSendPingTimeout {
		t.Fatalf("transport has no HTTP/2 health check: %+v", tr.HTTP2)
	}
	if got := pr.authChallenger.(*remoteAuthChallenger).client.Transport; got != pr.transport {
		t.Fatalf("auth challenger uses %v, want the upstream transport", got)
	}
}

func TestProxyingRegistryCloseClosesIdleUpstreamConnections(t *testing.T) {
	closed := make(chan struct{}, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	upstream.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	upstream.Start()
	t.Cleanup(upstream.Close)

	ctx := context.Background()
	driver := inmemory.New()
	local, err := storage.NewRegistry(ctx, driver)
	if err != nil {
		t.Fatal(err)
	}
	ttl := time.Duration(0)
	// The constructor fetches /v2/ from the upstream and leaves an idle connection.
	ns, err := NewRegistryPullThroughCache(ctx, local, driver, configuration.Proxy{RemoteURL: upstream.URL, TTL: &ttl})
	if err != nil {
		t.Fatal(err)
	}

	if err := ns.(*proxyingRegistry).Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("idle upstream connection was not closed by Close()")
	}
}

// blackholeProxy relays TCP traffic until frozen, then silently discards it
// without closing the connections, like a NAT that lost its state.
type blackholeProxy struct {
	ln     net.Listener
	target string
	frozen atomic.Bool
	wg     sync.WaitGroup

	mu     sync.Mutex
	conns  []net.Conn
	closed bool
}

func newBlackholeProxy(t *testing.T, target string) *blackholeProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &blackholeProxy{ln: ln, target: target}
	p.wg.Add(1)
	go p.serve()
	t.Cleanup(func() {
		ln.Close()
		p.mu.Lock()
		p.closed = true
		for _, c := range p.conns {
			c.Close()
		}
		p.mu.Unlock()
		p.wg.Wait()
	})
	return p
}

func (p *blackholeProxy) serve() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		u, err := net.Dial("tcp", p.target)
		if err != nil {
			c.Close()
			continue
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			c.Close()
			u.Close()
			return
		}
		p.conns = append(p.conns, c, u)
		p.wg.Add(2)
		p.mu.Unlock()
		go p.relay(u, c)
		go p.relay(c, u)
	}
}

func (p *blackholeProxy) relay(dst, src net.Conn) {
	defer p.wg.Done()
	defer dst.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		if p.frozen.Load() {
			continue
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			return
		}
	}
}

func TestUpstreamTransportDetectsDeadHTTP2Connection(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	t.Cleanup(upstream.Close)

	proxy := newBlackholeProxy(t, upstream.Listener.Addr().String())

	tr := newUpstreamTransport(time.Second, time.Second).(*http.Transport)
	tr.TLSClientConfig = upstream.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	t.Cleanup(tr.CloseIdleConnections)
	client := &http.Client{Transport: tr}
	url := "https://" + proxy.ln.Addr().String() + "/"

	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("got %s, want HTTP/2", resp.Proto)
	}

	proxy.frozen.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("request on a dead connection succeeded")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dead connection was not detected by the HTTP/2 health check: %v", err)
	}
}
