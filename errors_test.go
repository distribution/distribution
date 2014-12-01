package registry

import (
	"encoding/json"
	"testing"
)

// TestErrorCodes ensures that error code format, mappings and
// marshaling/unmarshaling. round trips are stable.
func TestErrorCodes(t *testing.T) {
	for ec := range errorCodeStrings {
		if ec.String() != errorCodeStrings[ec] {
			t.Fatalf("error code string incorrect: %q != %q", ec.String(), errorCodeStrings[ec])
		}

		if ec.Message() != errorCodesMessages[ec] {
			t.Fatalf("incorrect message for error code %v: %q != %q", ec, ec.Message(), errorCodesMessages[ec])
		}

		// Serialize the error code using the json library to ensure that we
		// get a string and it works round trip.
		p, err := json.Marshal(ec)

		if err != nil {
			t.Fatalf("error marshaling error code %v: %v", ec, err)
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
func TestErrorsManagement(t *testing.T) {
	var errs Errors

	errs.Push(ErrorCodeInvalidDigest)

	var detail DetailUnknownLayer
	detail.Unknown.BlobSum = "sometestblobsumdoesntmatter"

	errs.Push(ErrorCodeUnknownLayer, detail)

	p, err := json.Marshal(errs)

	if err != nil {
		t.Fatalf("error marashaling errors: %v", err)
	}

	expectedJSON := "{\"errors\":[{\"code\":\"INVALID_DIGEST\",\"message\":\"provided digest did not match uploaded content\"},{\"code\":\"UNKNOWN_LAYER\",\"message\":\"referenced layer not available\",\"detail\":{\"unknown\":{\"blobSum\":\"sometestblobsumdoesntmatter\"}}}]}"

	if string(p) != expectedJSON {
		t.Fatalf("unexpected json: %q != %q", string(p), expectedJSON)
	}

	errs.Clear()
	errs.Push(ErrorCodeUnknown)
	expectedJSON = "{\"errors\":[{\"code\":\"UNKNOWN\",\"message\":\"unknown error\"}]}"
	p, err = json.Marshal(errs)

	if err != nil {
		t.Fatalf("error marashaling errors: %v", err)
	}

	if string(p) != expectedJSON {
		t.Fatalf("unexpected json: %q != %q", string(p), expectedJSON)
	}
}
