package handlers

import (
	"net/http"
	"reflect"
	"testing"

	"golang.org/x/net/context"
)

func TestContextWithVars(t *testing.T) {
	var req http.Request
	vars := map[string]string{
		"foo": "asdf",
		"bar": "qwer",
	}

	getVarsFromRequest = func(r *http.Request) map[string]string {
		if r != &req {
			t.Fatalf("unexpected request: %v != %v", r, req)
		}

		return vars
	}

	ctx := contextWithVars(context.Background(), &req)
	for _, testcase := range []struct {
		key      string
		expected interface{}
	}{
		{
			key:      "vars",
			expected: vars,
		},
		{
			key:      "vars.foo",
			expected: "asdf",
		},
		{
			key:      "vars.bar",
			expected: "qwer",
		},
	} {
		v := ctx.Value(testcase.key)

		if !reflect.DeepEqual(v, testcase.expected) {
			t.Fatalf("%q: %v != %v", testcase.key, v, testcase.expected)
		}
	}
}
