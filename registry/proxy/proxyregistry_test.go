package proxy

import (
	"testing"
)

func TestProxyingRegistryCloseWithoutScheduler(t *testing.T) {
	pr := &proxyingRegistry{
		evictionController: nil,
	}

	// verify that `Close()` does not panic when the scheduler is nil
	err := pr.Close()
	if err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
}
