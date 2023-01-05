package configuration

import (
	"bytes"
	"testing"
)

// ParserFuzzer implements a fuzzer that targets Parser()
// nolint:deadcode
func FuzzConfigurationParse(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		rd := bytes.NewReader(data)
		_, _ = Parse(rd)
	})
}
