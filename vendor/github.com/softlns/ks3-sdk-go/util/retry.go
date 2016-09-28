package util

import (
	"net"
	"net/http"
)

// shouldRetry determines if we should retry the request.
//
func shouldRetry(r *http.Response, err error, numRetries int, maxRetries int) bool {
	// Once we've exceeded the max retry attempts, game over.
	if numRetries >= maxRetries {
		return false
	}

	// Always retry temporary network errors.
	if err, ok := err.(net.Error); ok && err.Temporary() {
		return true
	}

	// Always retry 5xx responses.
	if r != nil && r.StatusCode >= 500 {
		return true
	}

	// Always retry throttling exceptions.
	if err, ok := err.(ServiceError); ok && isThrottlingException(err) {
		return true
	}

	// Other classes of failures indicate a problem with the request. Retrying
	// won't help.
	return false
}

func isThrottlingException(err ServiceError) bool {
	switch err.ErrorCode() {
	case "Throttling", "ThrottlingException", "ProvisionedThroughputExceededException":
		return true
	default:
		return false
	}
}
