package checks

import (
	"errors"
	"github.com/docker/distribution/health"
	"net/http"
	"os"
)

// FileChecker checks the existence of a file and returns and error
// if the file exists, taking the application out of rotation
func FileChecker(f string) health.Checker {
	return health.CheckFunc(func() error {
		if _, err := os.Stat(f); err == nil {
			return errors.New("file exists")
		}
		return nil
	})
}

// HTTPChecker does a HEAD request and verifies if the HTTP status
// code return is a 200, taking the application out of rotation if
// otherwise
func HTTPChecker(r string) health.Checker {
	return health.CheckFunc(func() error {
		response, err := http.Head(r)
		if err != nil {
			return errors.New("error while checking: " + r)
		}
		if response.StatusCode != http.StatusOK {
			return errors.New("downstream service returned unexpected status: " + string(response.StatusCode))
		}
		return nil
	})
}
