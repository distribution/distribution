package v2

import (
	"testing"
)

// FuzzParseForwardedHeader implements a fuzzer
// that targets parseForwardedHeader
func FuzzParseForwardedHeader(f *testing.F) {
	f.Fuzz(func(t *testing.T, data string) {
		_, _, _ = parseForwardedHeader(data)
	})
}
