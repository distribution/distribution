package checks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/distribution/distribution/v3/health"
)

// FileChecker checks the existence of a file and returns an error
// if the file exists.
func FileChecker(f string) health.Checker {
	return health.CheckFunc(func(context.Context) error {
		absoluteFilePath, err := filepath.Abs(f)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %q: %v", f, err)
		}

		_, err = os.Stat(absoluteFilePath)
		if err == nil {
			return errors.New("file exists")
		} else if os.IsNotExist(err) {
			return nil
		}

		return err
	})
}

// HTTPChecker does a HEAD request and verifies that the HTTP status code
// returned matches statusCode.
func HTTPChecker(r string, statusCode int, timeout time.Duration, headers http.Header) health.Checker {
	return health.CheckFunc(func(ctx context.Context) error {
		client := http.Client{
			Timeout: timeout,
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, r, nil)
		if err != nil {
			return fmt.Errorf("%v: error creating request: %w", r, err)
		}
		for headerName, headerValues := range headers {
			for _, headerValue := range headerValues {
				req.Header.Add(headerName, headerValue)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%v: error while checking: %w", r, err)
		}
		defer response.Body.Close()
		if response.StatusCode != statusCode {
			return fmt.Errorf("%v: downstream service returned unexpected status: %d", r, response.StatusCode)
		}
		return nil
	})
}

// TCPChecker attempts to open a TCP connection.
func TCPChecker(addr string, timeout time.Duration) health.Checker {
	return health.CheckFunc(func(ctx context.Context) error {
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("%v: connection failed: %w", addr, err)
		}
		conn.Close()
		return nil
	})
}
