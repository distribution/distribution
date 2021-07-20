// +build gofuzz

package v2

// FuzzParseForwardedHeader implements a fuzzer
// that targets parseForwardedHeader
// Export before building
// nolint:deadcode
func fuzzParseForwardedHeader(data []byte) int {
	_, _, _ = parseForwardedHeader(string(data))
	return 1
}
