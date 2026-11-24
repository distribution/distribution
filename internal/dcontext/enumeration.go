package dcontext

import "context"

type enumerationKey struct{}

// EnumerationStatus is a context that records entries skipped by enumeration.
// Its status must only be read after the synchronous enumeration has finished.
type EnumerationStatus struct {
	context.Context
	incomplete bool
}

// WithEnumerationStatus tracks entries omitted without failing enumeration.
func WithEnumerationStatus(ctx context.Context) *EnumerationStatus {
	return &EnumerationStatus{Context: ctx}
}

func (e *EnumerationStatus) Value(key any) any {
	if _, ok := key.(enumerationKey); ok {
		return e
	}
	return e.Context.Value(key)
}

// Complete reports whether enumeration skipped no entries.
func (e *EnumerationStatus) Complete() bool { return !e.incomplete }

// EnumerationSkipped reports an entry omitted without failing enumeration.
// Callers not tracking enumeration status retain their usual behavior.
func EnumerationSkipped(ctx context.Context) {
	if status, ok := ctx.Value(enumerationKey{}).(*EnumerationStatus); ok {
		status.incomplete = true
	}
}
