package minimemcached

import "strconv"

const (
	asciiDel = 0x7f
)

func isLegalKey(key string) bool {
	if len(key) > maxKeyLength {
		return false
	}
	for _, k := range key {
		if k <= ' ' || k == asciiDel {
			return false
		}
	}
	return true
}

func isLegalValue(bytes int, value []byte) bool {
	return bytes == len(value)
}

func getNumericValueFromString(value string) (uint64, bool) {
	var numericValue uint64
	numericValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return numericValue, true
}

func getNumericValueFromByteArray(value []byte) (uint64, bool) {
	var numericValue uint64
	numericValue, err := strconv.ParseUint(string(value), 10, 64)
	if err != nil {
		return 0, false
	}
	return numericValue, true
}
