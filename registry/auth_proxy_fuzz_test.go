package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/docker/distribution/configuration"
)

// FuzzAuthProxyRequest drives the reverse proxy this fork puts in front of the
// authentication service.
//
// Threat model coverage (registry-threat-model.md), harness 5: the behaviour of
// the registry/auth_proxy.go reverse proxy when the authentication service
// answers badly or hostilely. The proxy is reached by any client that can talk to
// the registry -- it forwards /auth/token and nothing else -- and it is code the
// fork adds, so upstream review does not cover it.
//
// Two properties are asserted, both about what the authentication service is
// told rather than about what the client gets back:
//
//   - The outbound request must reach the configured URL's path and no other.
//     authProxyHandler overwrites r.Out.URL.Path with remote.Path, so the
//     inbound path should not be able to steer the request elsewhere in the auth
//     service -- to /metrics, or to a path that means something different.
//   - The client address the auth service sees must be the real one. This is the
//     property that spans the two forks: docker_auth reads the client address
//     from the header named by `real_ip_header`, which the module sets to
//     X-Forwarded-For, and takes the element at `real_ip_pos`, which defaults to
//     0 -- the first. Its ACL can match on `ip`. So if a client can put a value
//     at the front of that header, it decides what address the ACL is evaluated
//     against.
//
// A 502 from the proxy is a correct outcome when the upstream misbehaves and is
// not a finding; a panic or a hang is.
//
// This target performs a real HTTP round trip per iteration, and two of the
// hostile upstream behaviours below deliberately poison connection reuse -- a
// Content-Length that overstates the body, and `Connection: close`. Run it with
// bounded parallelism, or the workers exhaust the ephemeral port range and the
// engine reports a worker that "hung or terminated unexpectedly" rather than
// anything about the proxy:
//
//	go test ./registry/ -run FuzzAuthProxyRequest -fuzz FuzzAuthProxyRequest \
//	    -parallel 4 -fuzztime 120s
func FuzzAuthProxyRequest(f *testing.F) {
	upstream := newRecordingAuthService(f)
	defer upstream.Close()

	handler, remotePath, innerCalls := newAuthProxy(f, upstream.URL+"/auth")

	f.Add("/auth/token", "", "", "", byte(0))
	f.Add("/auth/token", "203.0.113.9", "", "", byte(0))
	f.Add("/auth/token", "203.0.113.9, 198.51.100.7", "", "", byte(0))
	f.Add("/auth/token", "127.0.0.1", "evil.example.com", "", byte(0))
	f.Add("/auth/token", "", "", "service=registry&scope=repository:a:pull", byte(0))
	f.Add("/auth/token/../metrics", "", "", "", byte(0))
	f.Add("/auth/token/..%2fmetrics", "", "", "", byte(0))
	f.Add("/auth/token/extra", "", "", "", byte(0))
	f.Add("/auth/token", "", "", "", byte(1))
	f.Add("/auth/token", "", "", "", byte(2))
	f.Add("/auth/token", "", "", "", byte(3))
	f.Add("/auth/token", "not an address", "", "", byte(0))
	f.Add("/auth/token", "203.0.113.9\t, x", "", "", byte(0))
	f.Add("/v2/", "203.0.113.9", "", "", byte(0))

	f.Fuzz(func(t *testing.T, path, forwardedFor, host, rawQuery string, upstreamBehaviour byte) {
		if len(path) > 4096 || len(forwardedFor) > 4096 ||
			len(host) > 1024 || len(rawQuery) > 4096 {
			return
		}
		if !sendableHeaderValue(forwardedFor) || !sendableHeaderValue(host) {
			// net/http will not transmit it, and its server side would reject it
			// before any of this code ran.
			return
		}
		if !strings.HasPrefix(path, "/") {
			return
		}

		target := path
		if rawQuery != "" {
			target += "?" + rawQuery
		}

		request, err := http.NewRequest(http.MethodGet, "http://registry.example.com"+target, nil)
		if err != nil {
			// An unparseable request line is not something this proxy can see:
			// net/http rejects it at the server boundary.
			return
		}

		const clientAddr = "192.0.2.11"
		request.RemoteAddr = clientAddr + ":54321"

		if forwardedFor != "" {
			request.Header.Set("X-Forwarded-For", forwardedFor)
		}
		if host != "" {
			request.Host = host
		}

		upstream.setBehaviour(upstreamBehaviour)
		upstream.reset()
		*innerCalls = 0

		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		received, forwarded := upstream.received()

		// The routing decision is the first thing to hold: /auth/token belongs to
		// the auth service and nothing else does. Checking which handler ran says
		// that directly, rather than inferring it from a status code -- the proxy
		// answers with several when it takes a request it cannot deliver, and an
		// outbound request that cannot be put on the wire (a raw space in the
		// query, say) is a correct refusal, not a routing mistake.
		//
		// The comparison is on the parsed path, which is what the handler
		// switches on. "/auth/token#x" parses to the path /auth/token with a
		// fragment, and a fragment never reaches a server anyway.
		if request.URL.Path == "/auth/token" {
			if *innerCalls != 0 {
				t.Fatalf("a request for %q (path %q) reached the registry handler instead of "+
					"the auth service\n\tquery: %q", path, request.URL.Path, rawQuery)
			}
		} else {
			if forwarded {
				t.Fatalf("a request for %q (path %q) was forwarded to the auth service; only "+
					"/auth/token belongs to it", path, request.URL.Path)
			}
			if *innerCalls != 1 {
				t.Fatalf("a request for %q (path %q) reached the registry handler %d times, "+
					"expected once", path, request.URL.Path, *innerCalls)
			}
		}

		if !forwarded {
			// The proxy took it and could not deliver it, which the assertions
			// above have already established is a refusal rather than a
			// misrouting.
			return
		}

		// The path is fixed by the configuration, not by the client.
		if received.path != remotePath {
			t.Fatalf("the auth service received the path %q for an inbound path of %q; "+
				"authProxyHandler sets r.Out.URL.Path from the configured URL, so the client "+
				"must not be able to choose it", received.path, path)
		}

		// The address the auth service will act on is the first element of
		// X-Forwarded-For, because that is where its real_ip_pos default points.
		if received.forwardedFor == "" {
			t.Fatalf("the auth service received no X-Forwarded-For; it resolves the client " +
				"address from that header and refuses the request without it")
		}

		first := strings.TrimSpace(strings.Split(received.forwardedFor, ",")[0])
		if first != clientAddr {
			t.Fatalf("the auth service would read the client address as %q, but the request came "+
				"from %q\n\tinbound X-Forwarded-For: %q\n\toutbound X-Forwarded-For: %q\n"+
				"docker_auth takes the element at real_ip_pos, which defaults to 0, and its ACL "+
				"can match on `ip`; a client that decides this value decides which rule applies",
				first, clientAddr, forwardedFor, received.forwardedFor)
		}

	})
}

// authServiceRequest is what the authentication service saw.
type authServiceRequest struct {
	path         string
	rawQuery     string
	forwardedFor string
	host         string
}

// recordingAuthService stands in for the authentication service and can answer
// badly on demand.
type recordingAuthService struct {
	*httptest.Server

	mu        sync.Mutex
	last      authServiceRequest
	got       bool
	behaviour byte
}

func newRecordingAuthService(f *testing.F) *recordingAuthService {
	f.Helper()

	service := &recordingAuthService{}
	service.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.mu.Lock()
		service.last = authServiceRequest{
			path:         r.URL.Path,
			rawQuery:     r.URL.RawQuery,
			forwardedFor: r.Header.Get("X-Forwarded-For"),
			host:         r.Host,
		}
		service.got = true
		behaviour := service.behaviour
		service.mu.Unlock()

		switch behaviour % 4 {
		case 0:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"token":"t","access_token":"t"}`)

		case 1:
			// A body that claims a length it does not deliver.
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "short")

		case 2:
			// Hop-by-hop and cookie headers, plus a redirect to somewhere else.
			w.Header().Set("Connection", "close")
			w.Header().Set("Set-Cookie", "session=1")
			w.Header().Set("Location", "http://127.0.0.1:1/")
			w.WriteHeader(http.StatusFound)

		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	return service
}

func (s *recordingAuthService) setBehaviour(behaviour byte) {
	s.mu.Lock()
	s.behaviour = behaviour
	s.mu.Unlock()
}

func (s *recordingAuthService) reset() {
	s.mu.Lock()
	s.got = false
	s.last = authServiceRequest{}
	s.mu.Unlock()
}

func (s *recordingAuthService) received() (authServiceRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last, s.got
}

// newAuthProxy builds the handler under test and returns the path the
// configuration pins the outbound request to.
func newAuthProxy(f *testing.F, upstreamURL string) (http.Handler, string, *int) {
	f.Helper()

	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		f.Fatalf("cannot parse the upstream URL: %v", err)
	}

	config := &configuration.Configuration{}
	config.Auth = configuration.Auth{
		"token": configuration.Parameters{
			"proxy": map[interface{}]interface{}{
				"url": upstreamURL,
			},
		},
	}

	// The registry's own handler, and a count of how often it ran: that is how
	// the harness tells routing from refusal.
	innerCalls := new(int)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*innerCalls++
		w.WriteHeader(http.StatusNotFound)
	})

	handler := authProxyHandler(context.Background(), config, inner)
	if handler == nil {
		f.Fatal("authProxyHandler returned no handler")
	}

	return handler, parsed.Path, innerCalls
}

// sendableHeaderValue reports whether net/http will put the value on the wire.
func sendableHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if c := value[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}
