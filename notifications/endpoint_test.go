package notifications

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewEndpoint(t *testing.T) {
	testEvents := []Event{
		createTestEvent("pull", "test", EventActionPull),
	}

	t.Run("synchronous", func(t *testing.T) {
		server, sendResponseCh := createBlockingServer(t)
		ep := createTestEndpoint(true, server.URL)

		writeDone := make(chan struct{})
		go func() {

			defer close(writeDone)

			err := ep.Write(testEvents...)
			if err != nil {
				t.Error(err)
			}
		}()

		// Verify that `server` received a request
		sendResponseCh <- struct{}{}

		// Since `Write` is blocking, the goroutine should not have returned yet.
		select {
		case <-writeDone:
			t.Errorf("goroutine should not have returned")
		default:
		}

		// `server` has not returned a response, so nothing has succeeded
		verifyNumSuccesses(t, ep, 0)

		// Now instruct `server` to return a response. Since this is a buffered channel, it blocks until `Write` has
		// been called
		sendResponseCh <- struct{}{}

		// wait until goroutine exits
		<-writeDone

		verifyNumSuccesses(t, ep, 1)
	})

	t.Run("asynchronous", func(t *testing.T) {
		server, sendResponseCh := createBlockingServer(t)
		ep := createTestEndpoint(false, server.URL)

		// since this is async, `Write` does not block
		err := ep.Write(testEvents...)
		if err != nil {
			t.Error(err)
		}

		// `Close` blocks until the sink if flushed.
		closeDone := make(chan struct{})
		go func() {
			defer close(closeDone)
			// flush the queue
			err = ep.Sink.Close()
			if err != nil {
				t.Error(err)
			}
		}()

		// verify that `server` received a request. It's important that the sink will flush at one point, otherwise
		// this might be blocked forever
		sendResponseCh <- struct{}{}

		verifyNumSuccesses(t, ep, 0)

		// now instruct `server` to return a response
		sendResponseCh <- struct{}{}

		<-closeDone

		verifyNumSuccesses(t, ep, 1)
	})

}

func verifyNumSuccesses(t *testing.T, ep *Endpoint, expected int) {
	successes := ep.metrics.EndpointMetrics.Successes
	if successes != expected {
		t.Errorf("should have received %d successful response, but got: %d", expected, successes)
	}
}

// createBlockingServer creates a test server that only responds when a value is sent to the returned channel.
// This is useful for testing sychronous vs asynchronous behavior.
func createBlockingServer(t *testing.T) (*httptest.Server, chan struct{}) {
	// NOTE(jmwong): It's important that this is an unbuffered channel, so that sends on the channel is blocked until
	// the server has received a request
	sendResponseCh := make(chan struct{})
	serverHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		<-sendResponseCh
		<-sendResponseCh
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewTLSServer(serverHandler)
	return server, sendResponseCh
}

func createTestEndpoint(sync bool, url string) *Endpoint {
	return NewEndpoint("test-new-endpoint", url, EndpointConfig{
		Sync: sync,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout:               10 * time.Second,
		testOnlyDoNotRegister: true,
	})
}
