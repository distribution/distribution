package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestReturns200IfThereAreNoChecks ensures that the result code of the health
// endpoint is 200 if there are not currently registered checks.
func TestReturns200IfThereAreNoChecks(t *testing.T) {
	recorder := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "https://fakeurl.com/debug/health", nil)
	if err != nil {
		t.Errorf("Failed to create request.")
	}

	StatusHandler(recorder, req)

	if recorder.Code != 200 {
		t.Errorf("Did not get a 200.")
	}
}

// TestReturns503IfThereAreErrorChecks ensures that the result code of the
// health endpoint is 503 if there are health checks with errors.
func TestReturns503IfThereAreErrorChecks(t *testing.T) {
	recorder := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "https://fakeurl.com/debug/health", nil)
	if err != nil {
		t.Errorf("Failed to create request.")
	}

	// Create a manual error
	Register("some_check", CheckFunc(func(context.Context) error {
		return errors.New("This Check did not succeed")
	}))

	StatusHandler(recorder, req)

	if recorder.Code != 503 {
		t.Errorf("Did not get a 503.")
	}
}

// TestHealthHandler ensures that our handler implementation correct protects
// the web application when things aren't so healthy.
func TestHealthHandler(t *testing.T) {
	// clear out existing checks.
	DefaultRegistry = NewRegistry()

	// protect an http server
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// wrap it in our health handler
	handler = Handler(handler)

	// use this swap check status
	updater := NewStatusUpdater()
	Register("test_check", updater)

	// now, create a test server
	server := httptest.NewServer(handler)

	checkUp := func(t *testing.T, message string) {
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("error getting success status: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("unexpected response code from server when %s: %d != %d", message, resp.StatusCode, http.StatusNoContent)
		}
		// NOTE(stevvooe): we really don't care about the body -- the format is
		// not standardized or supported, yet.
	}

	checkDown := func(t *testing.T, message string) {
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("error getting down status: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected response code from server when %s: %d != %d", message, resp.StatusCode, http.StatusServiceUnavailable)
		}
	}

	// server should be up
	checkUp(t, "initial health check")

	// now, we fail the health check
	updater.Update(fmt.Errorf("the server is now out of commission"))
	checkDown(t, "server should be down") // should be down

	// bring server back up
	updater.Update(nil)
	checkUp(t, "when server is back up") // now we should be back up.
}

func TestThresholdStatusUpdater(t *testing.T) {
	u := NewThresholdStatusUpdater(3)

	assertCheckOK := func() {
		t.Helper()
		if err := u.Check(context.Background()); err != nil {
			t.Errorf("u.Check() = %v; want nil", err)
		}
	}

	assertCheckErr := func(expected string) {
		t.Helper()
		if err := u.Check(context.Background()); err == nil || err.Error() != expected {
			t.Errorf("u.Check() = %v; want %v", err, expected)
		}
	}

	// Updater should report healthy until the threshold is reached.
	for i := 1; i <= 3; i++ {
		assertCheckOK()
		u.Update(fmt.Errorf("fake error %d", i))
	}
	assertCheckErr("fake error 3")

	// The threshold should reset after one successful update.
	u.Update(nil)
	assertCheckOK()
	u.Update(errors.New("first errored update after reset"))
	assertCheckOK()
	u.Update(nil)

	// pollingTerminatedErr should bypass the threshold.
	pte := pollingTerminatedErr{Err: errors.New("womp womp")}
	u.Update(pte)
	assertCheckErr(pte.Error())
}

func TestPoll(t *testing.T) {
	type ContextKey struct{}
	for _, threshold := range []int{0, 10} {
		t.Run(fmt.Sprintf("threshold=%d", threshold), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ContextKey{}, t.Name()))
			defer cancel()
			checkerCalled := make(chan struct{})
			checker := CheckFunc(func(ctx context.Context) error {
				if v, ok := ctx.Value(ContextKey{}).(string); !ok || v != t.Name() {
					t.Errorf("unexpected context passed into checker: got context with value %q, want %q", v, t.Name())
				}
				select {
				case <-checkerCalled:
				default:
					close(checkerCalled)
				}
				return nil
			})

			updater := NewThresholdStatusUpdater(threshold)
			pollReturned := make(chan struct{})
			go func() {
				Poll(ctx, updater, checker, 1*time.Millisecond)
				close(pollReturned)
			}()

			select {
			case <-checkerCalled:
			case <-time.After(1 * time.Second):
				t.Error("checker has not been polled")
			}

			cancel()

			select {
			case <-pollReturned:
			case <-time.After(1 * time.Second):
				t.Error("poll has not returned after context was canceled")
			}

			if err := updater.Check(context.Background()); !errors.Is(err, context.Canceled) {
				t.Errorf("updater.Check() = %v; want %v", err, context.Canceled)
			}
		})
	}
}
