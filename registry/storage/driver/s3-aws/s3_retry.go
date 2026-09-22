package s3

import (
	"context"
	"errors"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
)

// retryHTTP429 adds v1's status-based throttling retries without replacing the
// configured SDK retryer, its backoff, quota, or adaptive attempt tokens.
type retryHTTP429 struct {
	aws.Retryer
}

func (r retryHTTP429) IsErrorRetryable(err error) bool {
	if (retry.NoRetryCanceledError{}).IsErrorRetryable(err) == aws.FalseTernary {
		return false
	}
	var response interface{ HTTPStatusCode() int }
	if errors.As(err, &response) && response.HTTPStatusCode() == http.StatusTooManyRequests {
		return true
	}
	return r.Retryer.IsErrorRetryable(err)
}

func (r retryHTTP429) GetAttemptToken(ctx context.Context) (func(error) error, error) {
	if retryer, ok := r.Retryer.(aws.RetryerV2); ok {
		return retryer.GetAttemptToken(ctx)
	}
	return r.GetInitialToken(), nil
}
