package checks

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"fmt"
	"github.com/docker/distribution/health"
	"path/filepath"
)

// FileChecker checks the existence of a file and returns an error
// if the file exists.
func FileChecker(f string) health.Checker {
	return health.CheckFunc(func() error {
		// preconditon checks
		absoluteFilePath, err := filepath.Abs(f)
		if err != nil {
			return fmt.Errorf("get absolute path for %q error. %s", f, err)
		}
		_, err = os.Stat(absoluteFilePath)
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied for %s", f)
		}

		if err == nil {
			return errors.New("file exists")
		} else if os.IsNotExist(err) {
			return nil
		} else {
			return fmt.Errorf("%s", err)
		}
	})
}

// HTTPChecker does a HEAD request and verifies that the HTTP status code
// returned matches statusCode.
func HTTPChecker(r string, statusCode int, timeout time.Duration, headers http.Header) health.Checker {
	return health.CheckFunc(func() error {
		client := http.Client{
			Timeout: timeout,
		}
		req, err := http.NewRequest("HEAD", r, nil)
		if err != nil {
			return errors.New("error creating request: " + r)
		}
		for headerName, headerValues := range headers {
			for _, headerValue := range headerValues {
				req.Header.Add(headerName, headerValue)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			return errors.New("error while checking: " + r)
		}
		if response.StatusCode != statusCode {
			return errors.New("downstream service returned unexpected status: " + strconv.Itoa(response.StatusCode))
		}
		return nil
	})
}

// TCPChecker attempts to open a TCP connection.
func TCPChecker(addr string, timeout time.Duration) health.Checker {
	return health.CheckFunc(func() error {
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return errors.New("connection to " + addr + " failed")
		}
		conn.Close()
		return nil
	})
}
