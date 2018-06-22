package base

import (
	"fmt"
	"testing"
)

func TestGetLimitFromParameter(t *testing.T) {
	tests := []struct {
		Input    interface{}
		Expected uint64
		Min      uint64
		Default  uint64
		Err      error
	}{
		{"foo", 0, 5, 5, fmt.Errorf("parameter must be an integer, 'foo' invalid")},
		{"50", 50, 5, 5, nil},
		{"5", 25, 25, 50, nil}, // lower than Min returns Min
		{nil, 50, 25, 50, nil}, // nil returns default
		{812, 812, 25, 50, nil},
	}

	for _, item := range tests {
		t.Run(fmt.Sprint(item.Input), func(t *testing.T) {
			actual, err := GetLimitFromParameter(item.Input, item.Min, item.Default)

			if err != nil && item.Err != nil && err.Error() != item.Err.Error() {
				t.Fatalf("GetLimitFromParameter error, expected %#v got %#v", item.Err, err)
			}

			if actual != item.Expected {
				t.Fatalf("GetLimitFromParameter result error, expected %d got %d", item.Expected, actual)
			}
		})
	}
}
