package logcontext

import (
	"bytes"
	"strings"
	"testing"
)

// DecorationExpect defines the expected values a log decorated by a logcontext v2 decorator should have
type DecorationExpect struct {
	DecorationDisabled bool

	EntityGUID string
	EntityName string
	Hostname   string
	TraceID    string
	SpanID     string

	// decorator errors will result in an undecorated log message being printed
	// and an error message also being printed in a separate line
	DecoratorError error
}

// metadata indexes
const (
	entityguid = 1
	hostname   = 2
	traceid    = 3
	spanid     = 4
)

func entityname(vals []string) string {
	if len(vals) < 2 {
		return ""
	}

	return vals[len(vals)-2]
}

// ValidateDecoratedOutput is a testing tool that validates whether a bytes buffer decorated by a
// logcontext v2 decorator contains the values we expect it to.
func ValidateDecoratedOutput(t *testing.T, out *bytes.Buffer, expect *DecorationExpect) {
	actual := out.String()

	if expect.DecorationDisabled {
		if strings.Contains(actual, "NR-LINKING") {
			t.Fatal("log decoration was expected to be disabled, but were decorated anyway")
		} else {
			return
		}
	}

	if expect.DecoratorError != nil {
		if strings.Contains(actual, "NR-LINKING") {
			t.Fatal("logs should not be decorated when a decorator error occurs")
		}

		msg := expect.DecoratorError.Error()
		if !strings.Contains(actual, msg) {
			t.Fatalf("an error message debug log was expected, \"%s\", but was not found: %s", msg, actual)
		} else {
			return
		}
	}

	split := strings.Split(actual, "NR-LINKING")

	if len(split) != 2 {
		t.Fatalf("expected log decoration, but NR-LINKING data was missing: %s", actual)
	}

	linkingData := strings.Split(split[1], "|")

	if len(linkingData) < 5 {
		t.Errorf("linking data is missing required fields: %s", split[1])
	}

	if linkingData[entityguid] != expect.EntityGUID {
		t.Errorf("incorrect entity GUID; expect: %s actual: %s", expect.EntityGUID, linkingData[entityguid])
	}

	if linkingData[hostname] != expect.Hostname {
		t.Errorf("incorrect hostname; expect: %s actual: %s", expect.Hostname, linkingData[hostname])
	}

	if entityname(linkingData) != expect.EntityName {
		t.Errorf("incorrect entity name; expect: %s actual: %s", expect.EntityName, entityname(linkingData))
	}

	if expect.TraceID != "" && expect.SpanID != "" {
		if len(linkingData) < 7 {
			t.Errorf("transaction metadata is missing from linking data: %s", split[1])
		}

		if linkingData[traceid] != expect.TraceID {
			t.Errorf("incorrect traceID; expect: %s actual: %s", expect.TraceID, linkingData[traceid])
		}

		if linkingData[spanid] != expect.SpanID {
			t.Errorf("incorrect hostname; expect: %s actual: %s", expect.SpanID, linkingData[spanid])
		}
	}
}
