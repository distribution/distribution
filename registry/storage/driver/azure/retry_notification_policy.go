package azure

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
)

// Inspired by/credit goes to https://github.com/Azure/azure-storage-azcopy/blob/97ab7b92e766ad48965ac2933495dff1b04fb2a7/ste/xferRetryNotificationPolicy.go
type contextKey struct {
	name string
}

var (
	timeoutNotifyContextKey = contextKey{"timeoutNotify"}
	retryNotifyContextKey   = contextKey{"retryNotify"}
)

// retryNotificationReceiver should be implemented by code that wishes to be
// notified when a retry happens. Such code must register itself into the
// context, using withRetryNotification, so that the RetryNotificationPolicy
// can invoke the callback when necessary.
type retryNotificationReceiver interface {
	RetryCallback()
}

// withTimeoutNotification returns a context that contains indication of a
// timeout. The retryNotificationPolicy will then set the timeout flag when a
// timeout happens
func withTimeoutNotification(ctx context.Context, timeout *bool) context.Context {
	return context.WithValue(ctx, timeoutNotifyContextKey, timeout)
}

// withRetryNotifier returns a context that contains a retry notifier. The
// retryNotificationPolicy will then invoke the callback when a retry happens
func withRetryNotification(ctx context.Context, r retryNotificationReceiver) context.Context { // nolint: unused // may become useful at some point
	return context.WithValue(ctx, retryNotifyContextKey, r)
}

// PolicyFunc is a type that implements the Policy interface.
// Use this type when implementing a stateless policy as a first-class function.
type PolicyFunc func(*policy.Request) (*http.Response, error)

// Do implements the Policy interface on policyFunc.
func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
	return pf(req)
}

func newRetryNotificationPolicy() policy.Policy {
	getErrorCode := func(resp *http.Response) string {
		// NOTE(prozlach): This is a hacky way to handle all possible cases of
		// emitting error by the Azure backend.
		// In theory we could look just at `x-ms-error-code` HTTP header, but
		// in practice Azure SDK also looks at the body and decodes it as JSON
		// or XML in case when the header is absent.
		// So the idea is to piggy-back on the runtime.NewResponseError that
		// will do the proper decoding for us and just return the ErrorCode
		// field instead.
		return runtime.NewResponseError(resp).(*azcore.ResponseError).ErrorCode
	}

	return PolicyFunc(func(req *policy.Request) (*http.Response, error) {
		response, err := req.Next() // Make the request

		if response == nil {
			return nil, err
		}

		switch response.StatusCode {
		case http.StatusServiceUnavailable:
			// Grab the notification callback out of the context and, if its there, call it
			if notifier, ok := req.Raw().Context().Value(retryNotifyContextKey).(retryNotificationReceiver); ok {
				notifier.RetryCallback()
			}
		case http.StatusInternalServerError:
			errorCodeHeader := getErrorCode(response)
			if bloberror.Code(errorCodeHeader) != bloberror.OperationTimedOut {
				break
			}

			if timeout, ok := req.Raw().Context().Value(timeoutNotifyContextKey).(*bool); ok {
				*timeout = true
			}
		}
		return response, err
	})
}
