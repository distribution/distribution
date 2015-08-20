package checks

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/docker/distribution/health"
)

// FileChecker checks the existence of a file and returns an error
// if the file exists.
func FileChecker(f string) health.Checker {
	return health.CheckFunc(func() error {
		if _, err := os.Stat(f); err == nil {
			return errors.New("file exists")
		}
		return nil
	})
}

// HTTPChecker does a HEAD request and verifies that the HTTP status code
// returned matches statusCode.
func HTTPChecker(r string, statusCode int, timeout time.Duration) health.Checker {
	return health.CheckFunc(func() error {
		client := http.Client{
			Timeout: timeout,
		}
		response, err := client.Head(r)
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
