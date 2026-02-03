package dcontext

import "context"

// DetachedContext returns a context that won't be canceled when the parent
// context is canceled. This is useful for operations that need to complete
// even after the HTTP request context is canceled (e.g., notifications,
// background cleanup, cache writes).
//
// The detached context preserves all values from the parent context (logger,
// request ID, etc.) but removes cancellation/deadline behavior.
//
// Example usage:
//
//	detachedCtx := dcontext.DetachedContext(ctx)
//	// Use detachedCtx for operations that must complete even if ctx is canceled
//	if err := someOperation(detachedCtx); err != nil {
//		GetLogger(ctx).Errorf("operation failed: %v", err)
//	}
func DetachedContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
