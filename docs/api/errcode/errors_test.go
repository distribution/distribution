package errcode

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// TestErrorCodes ensures that error code format, mappings and
// marshaling/unmarshaling. round trips are stable.
func TestErrorCodes(t *testing.T) {
	if len(errorCodeToDescriptors) == 0 {
		t.Fatal("errors aren't loaded!")
	}

	for ec, desc := range errorCodeToDescriptors {
		if ec != desc.Code {
			t.Fatalf("error code in descriptor isn't correct, %q != %q", ec, desc.Code)
		}

		if idToDescriptors[desc.Value].Code != ec {
			t.Fatalf("error code in idToDesc isn't correct, %q != %q", idToDescriptors[desc.Value].Code, ec)
		}

		if ec.Message() != desc.Message {
			t.Fatalf("ec.Message doesn't mtach desc.Message: %q != %q", ec.Message(), desc.Message)
		}

		// Test (de)serializing the ErrorCode
		p, err := json.Marshal(ec)
		if err != nil {
			t.Fatalf("couldn't marshal ec %v: %v", ec, err)
		}

		if len(p) <= 0 {
			t.Fatalf("expected content in marshaled before for error code %v", ec)
		}

		// First, unmarshal to interface and ensure we have a string.
		var ecUnspecified interface{}
		if err := json.Unmarshal(p, &ecUnspecified); err != nil {
			t.Fatalf("error unmarshaling error code %v: %v", ec, err)
		}

		if _, ok := ecUnspecified.(string); !ok {
			t.Fatalf("expected a string for error code %v on unmarshal got a %T", ec, ecUnspecified)
		}

		// Now, unmarshal with the error code type and ensure they are equal
		var ecUnmarshaled ErrorCode
		if err := json.Unmarshal(p, &ecUnmarshaled); err != nil {
			t.Fatalf("error unmarshaling error code %v: %v", ec, err)
		}

		if ecUnmarshaled != ec {
			t.Fatalf("unexpected error code during error code marshal/unmarshal: %v != %v", ecUnmarshaled, ec)
		}
	}

}

// TestErrorsManagement does a quick check of the Errors type to ensure that
// members are properly pushed and marshaled.
var ErrorCodeTest1 = Register("v2.errors", ErrorDescriptor{
	Value:          "TEST1",
	Message:        "test error 1",
	Description:    `Just a test message #1.`,
	HTTPStatusCode: http.StatusInternalServerError,
})

var ErrorCodeTest2 = Register("v2.errors", ErrorDescriptor{
	Value:          "TEST2",
	Message:        "test error 2",
	Description:    `Just a test message #2.`,
	HTTPStatusCode: http.StatusNotFound,
})

func TestErrorsManagement(t *testing.T) {
	var errs Errors

	errs = append(errs, NewError(ErrorCodeTest1))
	errs = append(errs, NewError(ErrorCodeTest2,
		map[string]interface{}{"digest": "sometestblobsumdoesntmatter"}))

	p, err := json.Marshal(errs)

	if err != nil {
		t.Fatalf("error marashaling errors: %v", err)
	}

	expectedJSON := "[{\"code\":\"TEST1\"},{\"code\":\"TEST2\",\"detail\":{\"digest\":\"sometestblobsumdoesntmatter\"}}]"

	if string(p) != expectedJSON {
		t.Fatalf("unexpected json: %q != %q", string(p), expectedJSON)
	}

	// Now test the reverse
	var unmarshaled Errors
	if err := json.Unmarshal(p, &unmarshaled); err != nil {
		t.Fatalf("unexpected error unmarshaling error envelope: %v", err)
	}

	if !reflect.DeepEqual(unmarshaled, errs) {
		t.Fatalf("errors not equal after round trip:\nunmarshaled:\n%#v\n\nerrs:\n%#v", unmarshaled, errs)
	}

	// Test again with a single value this time
	errs = Errors{NewError(ErrorCodeUnknown)}
	expectedJSON = "[{\"code\":\"UNKNOWN\"}]"
	p, err = json.Marshal(errs)

	if err != nil {
		t.Fatalf("error marashaling errors: %v", err)
	}

	if string(p) != expectedJSON {
		t.Fatalf("unexpected json: %q != %q", string(p), expectedJSON)
	}

	// Now test the reverse
	unmarshaled = nil
	if err := json.Unmarshal(p, &unmarshaled); err != nil {
		t.Fatalf("unexpected error unmarshaling error envelope: %v", err)
	}

	if !reflect.DeepEqual(unmarshaled, errs) {
		t.Fatalf("errors not equal after round trip:\nunmarshaled:\n%#v\n\nerrs:\n%#v", unmarshaled, errs)
	}

}
