package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

var (
	mutex            sync.RWMutex
	registeredChecks = make(map[string]Checker)
)

// Checker is the interface for a Health Checker
type Checker interface {
	// Check returns nil if the service is okay.
	Check() error
}

// CheckFunc is a convenience type to create functions that implement
// the Checker interface
type CheckFunc func() error

// Check Implements the Checker interface to allow for any func() error method
// to be passed as a Checker
func (cf CheckFunc) Check() error {
	return cf()
}

// Updater implements a health check that is explicitly set.
type Updater interface {
	Checker

	// Update updates the current status of the health check.
	Update(status error)
}

// updater implements Checker and Updater, providing an asynchronous Update
// method.
// This allows us to have a Checker that returns the Check() call immediately
// not blocking on a potentially expensive check.
type updater struct {
	mu     sync.Mutex
	status error
}

// Check implements the Checker interface
func (u *updater) Check() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.status
}

// Update implements the Updater interface, allowing asynchronous access to
// the status of a Checker.
func (u *updater) Update(status error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.status = status
}

// NewStatusUpdater returns a new updater
func NewStatusUpdater() Updater {
	return &updater{}
}

// thresholdUpdater implements Checker and Updater, providing an asynchronous Update
// method.
// This allows us to have a Checker that returns the Check() call immediately
// not blocking on a potentially expensive check.
type thresholdUpdater struct {
	mu        sync.Mutex
	status    error
	threshold int
	count     int
}

// Check implements the Checker interface
func (tu *thresholdUpdater) Check() error {
	tu.mu.Lock()
	defer tu.mu.Unlock()

	if tu.count >= tu.threshold {
		return tu.status
	}

	return nil
}

// thresholdUpdater implements the Updater interface, allowing asynchronous
// access to the status of a Checker.
func (tu *thresholdUpdater) Update(status error) {
	tu.mu.Lock()
	defer tu.mu.Unlock()

	if status == nil {
		tu.count = 0
	} else if tu.count < tu.threshold {
		tu.count++
	}

	tu.status = status
}

// NewThresholdStatusUpdater returns a new thresholdUpdater
func NewThresholdStatusUpdater(t int) Updater {
	return &thresholdUpdater{threshold: t}
}

// PeriodicChecker wraps an updater to provide a periodic checker
func PeriodicChecker(check Checker, period time.Duration) Checker {
	u := NewStatusUpdater()
	go func() {
		t := time.NewTicker(period)
		for {
			<-t.C
			u.Update(check.Check())
		}
	}()

	return u
}

// PeriodicThresholdChecker wraps an updater to provide a periodic checker that
// uses a threshold before it changes status
func PeriodicThresholdChecker(check Checker, period time.Duration, threshold int) Checker {
	tu := NewThresholdStatusUpdater(threshold)
	go func() {
		t := time.NewTicker(period)
		for {
			<-t.C
			tu.Update(check.Check())
		}
	}()

	return tu
}

// CheckStatus returns a map with all the current health check errors
func CheckStatus() map[string]string {
	mutex.RLock()
	defer mutex.RUnlock()
	statusKeys := make(map[string]string)
	for k, v := range registeredChecks {
		err := v.Check()
		if err != nil {
			statusKeys[k] = err.Error()
		}
	}

	return statusKeys
}

// Register associates the checker with the provided name. We allow
// overwrites to a specific check status.
func Register(name string, check Checker) {
	mutex.Lock()
	defer mutex.Unlock()
	_, ok := registeredChecks[name]
	if ok {
		panic("Check already exists: " + name)
	}
	registeredChecks[name] = check
}

// RegisterFunc allows the convenience of registering a checker directly
// from an arbitrary func() error
func RegisterFunc(name string, check func() error) {
	Register(name, CheckFunc(check))
}

// RegisterPeriodicFunc allows the convenience of registering a PeriodicChecker
// from an arbitrary func() error
func RegisterPeriodicFunc(name string, check func() error, period time.Duration) {
	Register(name, PeriodicChecker(CheckFunc(check), period))
}

// RegisterPeriodicThresholdFunc allows the convenience of registering a
// PeriodicChecker from an arbitrary func() error
func RegisterPeriodicThresholdFunc(name string, check func() error, period time.Duration, threshold int) {
	Register(name, PeriodicThresholdChecker(CheckFunc(check), period, threshold))
}

// StatusHandler returns a JSON blob with all the currently registered Health Checks
// and their corresponding status.
// Returns 503 if any Error status exists, 200 otherwise
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		checksStatus := CheckStatus()
		// If there is an error, return 503
		if len(checksStatus) != 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		encoder := json.NewEncoder(w)
		err := encoder.Encode(checksStatus)

		// Parsing of the JSON failed. Returning generic error message
		if err != nil {
			encoder.Encode(struct {
				ServerError string `json:"server_error"`
			}{
				ServerError: "Could not parse error message",
			})
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

// Registers global /debug/health api endpoint
func init() {
	http.HandleFunc("/debug/health", StatusHandler)
}
