package dcontext

import (
	"runtime"
	"testing"
	"time"
)

// TestWithTrace ensures that tracing has the expected values in the context.
func TestWithTrace(t *testing.T) {
	t.Parallel()
	pc, file, _, _ := runtime.Caller(0) // get current caller.
	f := runtime.FuncForPC(pc)

	base := []valueTestCase{
		{
			key:           "trace.id",
			notnilorempty: true,
		},

		{
			key:           "trace.file",
			expected:      file,
			notnilorempty: true,
		},
		{
			key:           "trace.line",
			notnilorempty: true,
		},
		{
			key:           "trace.start",
			notnilorempty: true,
		},
	}

	ctx, done := WithTrace(Background())
	t.Cleanup(func() { done("this will be emitted at end of test") })

	tests := append(base, valueTestCase{
		key:      "trace.func",
		expected: f.Name(),
	})
	for _, tc := range tests {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			v := ctx.Value(tc.key)
			if tc.notnilorempty {
				if v == nil || v == "" {
					t.Fatalf("value was nil or empty: %#v", v)
				}
				return
			}

			if v != tc.expected {
				t.Fatalf("unexpected value: %v != %v", v, tc.expected)
			}
		})
	}

	tracedFn := func() {
		parentID := ctx.Value("trace.id") // ensure the parent trace id is correct.

		pc, _, _, _ := runtime.Caller(0) // get current caller.
		f := runtime.FuncForPC(pc)
		ctx, done := WithTrace(ctx)
		defer done("this should be subordinate to the other trace")
		time.Sleep(time.Second)
		tests := append(base, valueTestCase{
			key:      "trace.func",
			expected: f.Name(),
		}, valueTestCase{
			key:      "trace.parent.id",
			expected: parentID,
		})
		for _, tc := range tests {
			tc := tc
			t.Run(tc.key, func(t *testing.T) {
				t.Parallel()
				v := ctx.Value(tc.key)
				if tc.notnilorempty {
					if v == nil || v == "" {
						t.Fatalf("value was nil or empty: %#v", v)
					}
					return
				}

				if v != tc.expected {
					t.Fatalf("unexpected value: %v != %v", v, tc.expected)
				}
			})
		}
	}
	tracedFn()

	time.Sleep(time.Second)
}

type valueTestCase struct {
	key           string
	expected      interface{}
	notnilorempty bool // just check not empty/not nil
}
