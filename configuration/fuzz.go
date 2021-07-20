// +build gofuzz

package configuration

import (
	"bytes"
)

// ParserFuzzer implements a fuzzer that targets Parser()
// Export before building
// nolint:deadcode
func parserFuzzer(data []byte) int {
	rd := bytes.NewReader(data)
	_, _ = Parse(rd)
	return 1
}
