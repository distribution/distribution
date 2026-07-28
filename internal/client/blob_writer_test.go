package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/testutil"
)

// Test implements distribution.BlobWriter
var _ distribution.BlobWriter = &httpBlobUpload{}

func TestUploadReadFrom(t *testing.T) {
	_, b := newRandomBlob(64)
	repo := "test/upload/readfrom"
	locationPath := fmt.Sprintf("/v2/%s/uploads/testid", repo)

	m := testutil.RequestResponseMap([]testutil.RequestResponseMapping{
		{
			Request: testutil.Request{
				Method: http.MethodGet,
				Route:  "/v2/",
			},
			Response: testutil.Response{
				StatusCode: http.StatusOK,
				Headers: http.Header(map[string][]string{
					"Docker-Distribution-API-Version": {"registry/2.0"},
				}),
			},
		},
		// Test Valid case
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {locationPath},
					"Range":              {"0-63"},
				}),
			},
		},
		// Test invalid range
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {locationPath},
					"Range":              {""},
				}),
			},
		},
		// Test 404
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusNotFound,
			},
		},
		// Test 400 valid json
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusBadRequest,
				Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
				Body: []byte(`
					{ "errors":
						[
							{
								"code": "BLOB_UPLOAD_INVALID",
								"message": "blob upload invalid",
								"detail": "more detail"
							}
						]
					} `),
			},
		},
		// Test 400 invalid json
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusBadRequest,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte("something bad happened"),
			},
		},
		// Test 500
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusInternalServerError,
			},
		},
	})

	e, c := testServer(m)
	defer c()

	blobUpload := &httpBlobUpload{
		ctx:    context.Background(),
		client: &http.Client{},
	}

	// Valid case
	blobUpload.location = e + locationPath
	n, err := blobUpload.ReadFrom(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Error calling ReadFrom: %s", err)
	}
	if n != 64 {
		t.Fatalf("Wrong length returned from ReadFrom: %d, expected 64", n)
	}

	// Bad range
	blobUpload.location = e + locationPath
	_, err = blobUpload.ReadFrom(bytes.NewReader(b))
	if err == nil {
		t.Fatal("Expected error when bad range received")
	}

	// 404
	blobUpload.location = e + locationPath
	_, err = blobUpload.ReadFrom(bytes.NewReader(b))
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("Wrong error thrown: %s, expected %s", err, distribution.ErrBlobUploadUnknown)
	}

	// 400 valid json
	blobUpload.location = e + locationPath
	_, err = blobUpload.ReadFrom(bytes.NewReader(b))
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(errcode.Errors); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else if len(uploadErr) != 1 {
		t.Fatalf("Unexpected number of errors: %d, expected 1", len(uploadErr))
	} else {
		v2Err, ok := uploadErr[0].(errcode.Error)
		if !ok {
			t.Fatalf("Not an 'Error' type: %#v", uploadErr[0])
		}
		if v2Err.Code != errcode.ErrorCodeBlobUploadInvalid {
			t.Fatalf("Unexpected error code: %s, expected %d", v2Err.Code.String(), errcode.ErrorCodeBlobUploadInvalid)
		}
		if expected := "blob upload invalid"; v2Err.Message != expected {
			t.Fatalf("Unexpected error message: %q, expected %q", v2Err.Message, expected)
		}
		if expected := "more detail"; v2Err.Detail.(string) != expected {
			t.Fatalf("Unexpected error message: %q, expected %q", v2Err.Detail.(string), expected)
		}
	}

	// 400 invalid json
	blobUpload.location = e + locationPath
	_, err = blobUpload.ReadFrom(bytes.NewReader(b))
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(*UnexpectedHTTPResponseError); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else {
		respStr := string(uploadErr.Response)
		if expected := "something bad happened"; respStr != expected {
			t.Fatalf("Unexpected response string: %s, expected: %s", respStr, expected)
		}
	}

	// 500
	blobUpload.location = e + locationPath
	_, err = blobUpload.ReadFrom(bytes.NewReader(b))
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(*UnexpectedHTTPStatusError); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else if expected := "500 " + http.StatusText(http.StatusInternalServerError); uploadErr.Status != expected {
		t.Fatalf("Unexpected response status: %s, expected %s", uploadErr.Status, expected)
	}
}

func TestUploadSize(t *testing.T) {
	_, b := newRandomBlob(64)
	readFromLocationPath := "/v2/test/upload/readfrom/uploads/testid"
	writeLocationPath := "/v2/test/upload/readfrom/uploads/testid"

	m := testutil.RequestResponseMap([]testutil.RequestResponseMapping{
		{
			Request: testutil.Request{
				Method: http.MethodGet,
				Route:  "/v2/",
			},
			Response: testutil.Response{
				StatusCode: http.StatusOK,
				Headers: http.Header(map[string][]string{
					"Docker-Distribution-API-Version": {"registry/2.0"},
				}),
			},
		},
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  readFromLocationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {readFromLocationPath},
					"Range":              {"0-63"},
				}),
			},
		},
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  writeLocationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {writeLocationPath},
					"Range":              {"0-63"},
				}),
			},
		},
	})

	e, c := testServer(m)
	defer c()

	// Writing with ReadFrom
	blobUpload := &httpBlobUpload{
		ctx:      context.Background(),
		client:   &http.Client{},
		location: e + readFromLocationPath,
	}

	if blobUpload.Size() != 0 {
		t.Fatalf("Wrong size returned from Size: %d, expected 0", blobUpload.Size())
	}

	_, err := blobUpload.ReadFrom(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Error calling ReadFrom: %s", err)
	}

	if blobUpload.Size() != 64 {
		t.Fatalf("Wrong size returned from Size: %d, expected 64", blobUpload.Size())
	}

	// Writing with Write
	blobUpload = &httpBlobUpload{
		ctx:      context.Background(),
		client:   &http.Client{},
		location: e + writeLocationPath,
	}

	_, err = blobUpload.Write(b)
	if err != nil {
		t.Fatalf("Error calling Write: %s", err)
	}

	if blobUpload.Size() != 64 {
		t.Fatalf("Wrong size returned from Size: %d, expected 64", blobUpload.Size())
	}
}

func TestUploadWrite(t *testing.T) {
	_, b := newRandomBlob(64)
	repo := "test/upload/write"
	locationPath := fmt.Sprintf("/v2/%s/uploads/testid", repo)

	m := testutil.RequestResponseMap([]testutil.RequestResponseMapping{
		{
			Request: testutil.Request{
				Method: http.MethodGet,
				Route:  "/v2/",
			},
			Response: testutil.Response{
				StatusCode: http.StatusOK,
				Headers: http.Header(map[string][]string{
					"Docker-Distribution-API-Version": {"registry/2.0"},
				}),
			},
		},
		// Test Valid case
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {locationPath},
					"Range":              {"0-63"},
				}),
			},
		},
		// Test invalid range
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {locationPath},
					"Range":              {""},
				}),
			},
		},
		// Test 404
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusNotFound,
			},
		},
		// Test 400 valid json
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusBadRequest,
				Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
				Body: []byte(`
					{ "errors":
						[
							{
								"code": "BLOB_UPLOAD_INVALID",
								"message": "blob upload invalid",
								"detail": "more detail"
							}
						]
					} `),
			},
		},
		// Test 400 invalid json
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusBadRequest,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte("something bad happened"),
			},
		},
		// Test 500
		{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b,
			},
			Response: testutil.Response{
				StatusCode: http.StatusInternalServerError,
			},
		},
	})

	e, c := testServer(m)
	defer c()

	blobUpload := &httpBlobUpload{
		ctx:    context.Background(),
		client: &http.Client{},
	}

	// Valid case
	blobUpload.location = e + locationPath
	n, err := blobUpload.Write(b)
	if err != nil {
		t.Fatalf("Error calling Write: %s", err)
	}
	if n != 64 {
		t.Fatalf("Wrong length returned from Write: %d, expected 64", n)
	}

	// Bad range
	blobUpload.location = e + locationPath
	_, err = blobUpload.Write(b)
	if err == nil {
		t.Fatal("Expected error when bad range received")
	}

	// 404
	blobUpload.location = e + locationPath
	_, err = blobUpload.Write(b)
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("Wrong error thrown: %s, expected %s", err, distribution.ErrBlobUploadUnknown)
	}

	// 400 valid json
	blobUpload.location = e + locationPath
	_, err = blobUpload.Write(b)
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(errcode.Errors); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else if len(uploadErr) != 1 {
		t.Fatalf("Unexpected number of errors: %d, expected 1", len(uploadErr))
	} else {
		v2Err, ok := uploadErr[0].(errcode.Error)
		if !ok {
			t.Fatalf("Not an 'Error' type: %#v", uploadErr[0])
		}
		if v2Err.Code != errcode.ErrorCodeBlobUploadInvalid {
			t.Fatalf("Unexpected error code: %s, expected %d", v2Err.Code.String(), errcode.ErrorCodeBlobUploadInvalid)
		}
		if expected := "blob upload invalid"; v2Err.Message != expected {
			t.Fatalf("Unexpected error message: %q, expected %q", v2Err.Message, expected)
		}
		if expected := "more detail"; v2Err.Detail.(string) != expected {
			t.Fatalf("Unexpected error message: %q, expected %q", v2Err.Detail.(string), expected)
		}
	}

	// 400 invalid json
	blobUpload.location = e + locationPath
	_, err = blobUpload.Write(b)
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(*UnexpectedHTTPResponseError); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else {
		respStr := string(uploadErr.Response)
		if expected := "something bad happened"; respStr != expected {
			t.Fatalf("Unexpected response string: %s, expected: %s", respStr, expected)
		}
	}

	// 500
	blobUpload.location = e + locationPath
	_, err = blobUpload.Write(b)
	if err == nil {
		t.Fatal("Expected error when not found")
	}
	if uploadErr, ok := err.(*UnexpectedHTTPStatusError); !ok {
		t.Fatalf("Wrong error type %T: %s", err, err)
	} else if expected := "500 " + http.StatusText(http.StatusInternalServerError); uploadErr.Status != expected {
		t.Fatalf("Unexpected response status: %s, expected %s", uploadErr.Status, expected)
	}
}

// TestUploadHandleErrorResponseNotFound covers
// https://github.com/distribution/distribution/issues/2512: a 404 response
// can mean the upload session is gone (BLOB_UPLOAD_UNKNOWN), but it can also
// carry a more specific error such as BLOB_UPLOAD_INVALID. Only the former
// should collapse to the generic distribution.ErrBlobUploadUnknown sentinel;
// the latter must be passed through so the registry's actual error message
// reaches the caller.
func TestUploadHandleErrorResponseNotFound(t *testing.T) {
	hbu := &httpBlobUpload{}

	newResponse := func(body string) *http.Response {
		header := http.Header{}
		if body != "" {
			header.Set("Content-Type", "application/json")
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}

	// No body at all: fall back to the generic sentinel.
	if err := hbu.handleErrorResponse(newResponse("")); err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("expected ErrBlobUploadUnknown, got %v", err)
	}

	// Explicit BLOB_UPLOAD_UNKNOWN: still the generic sentinel.
	err := hbu.handleErrorResponse(newResponse(`{"errors":[{"code":"BLOB_UPLOAD_UNKNOWN","message":"blob upload unknown to registry"}]}`))
	if err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("expected ErrBlobUploadUnknown, got %v", err)
	}

	// BLOB_UPLOAD_INVALID must not be masked by the generic sentinel.
	err = hbu.handleErrorResponse(newResponse(`{"errors":[{"code":"BLOB_UPLOAD_INVALID","message":"blob upload invalid","detail":"digest mismatch"}]}`))
	errs, ok := err.(errcode.Errors)
	if !ok || len(errs) != 1 {
		t.Fatalf("expected errcode.Errors with one entry, got %T: %v", err, err)
	}
	e, ok := errs[0].(errcode.Error)
	if !ok || e.Code != errcode.ErrorCodeBlobUploadInvalid {
		t.Fatalf("expected BLOB_UPLOAD_INVALID error, got %#v", errs[0])
	}
	if expected := "blob upload invalid"; e.Message != expected {
		t.Fatalf("unexpected error message: %q, expected %q", e.Message, expected)
	}

	// BLOB_UPLOAD_INVALID with no detail and the default message unmarshals
	// to a bare errcode.ErrorCode rather than errcode.Error (see
	// errcode.Errors.UnmarshalJSON), so it must be recognized too.
	err = hbu.handleErrorResponse(newResponse(`{"errors":[{"code":"BLOB_UPLOAD_INVALID","message":"blob upload invalid"}]}`))
	errs, ok = err.(errcode.Errors)
	if !ok || len(errs) != 1 {
		t.Fatalf("expected errcode.Errors with one entry, got %T: %v", err, err)
	}
	if ec, ok := errs[0].(errcode.ErrorCode); !ok || ec != errcode.ErrorCodeBlobUploadInvalid {
		t.Fatalf("expected BLOB_UPLOAD_INVALID error code, got %#v", errs[0])
	}
}
