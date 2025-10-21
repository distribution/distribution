package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// tests the case of sending only the bytes that we're limiting on
func TestUploadLimitRange(t *testing.T) {
	const numberOfBlobs = 10
	const blobSize = 5
	const lastBlobOffset = 2

	_, b := newRandomBlob(numberOfBlobs*5 + 2)
	repo := "test/upload/write"
	locationPath := fmt.Sprintf("/v2/%s/uploads/testid", repo)
	requests := []testutil.RequestResponseMapping{
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
	}

	for blob := 0; blob < numberOfBlobs; blob++ {
		start := blob * blobSize
		end := ((blob + 1) * blobSize) - 1

		requests = append(requests, testutil.RequestResponseMapping{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  locationPath,
				Body:   b[start : end+1],
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
					"Location":           {locationPath},
					"Range":              {fmt.Sprintf("%d-%d", start, end)},
				}),
			},
		})
	}

	requests = append(requests, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPatch,
			Route:  locationPath,
			Body:   b[numberOfBlobs*blobSize:],
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Docker-Upload-UUID": {"46603072-7a1b-4b41-98f9-fd8a7da89f9b"},
				"Location":           {locationPath},
				"Range":              {fmt.Sprintf("%d-%d", numberOfBlobs*blobSize, len(b)-1)},
			}),
		},
	})

	t.Run("reader chunked upload", func(t *testing.T) {
		m := testutil.RequestResponseMap(requests)
		e, c := testServer(m)
		defer c()

		blobUpload := &httpBlobUpload{
			ctx:      context.Background(),
			client:   &http.Client{},
			maxRange: int64(blobSize),
		}

		reader := bytes.NewBuffer(b)
		for i := 0; i < numberOfBlobs; i++ {
			blobUpload.location = e + locationPath
			n, err := blobUpload.ReadFrom(reader)
			if err != nil {
				t.Fatalf("Error calling Write: %s", err)
			}

			if n != blobSize {
				t.Fatalf("Unexpected n %v != %v blobSize", n, blobSize)
			}
		}

		n, err := blobUpload.ReadFrom(reader)
		if err != nil {
			t.Fatalf("Error calling Write: %s", err)
		}

		if n != lastBlobOffset {
			t.Fatalf("Expected last write to have written %v but wrote %v", lastBlobOffset, n)
		}

		_, err = reader.Read([]byte{0, 0, 0})
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Expected io.EOF when reading blob as the test should've read the whole thing")
		}
	})

	t.Run("buffer chunked upload", func(t *testing.T) {
		buff := b
		m := testutil.RequestResponseMap(requests)
		e, c := testServer(m)
		defer c()

		blobUpload := &httpBlobUpload{
			ctx:      context.Background(),
			client:   &http.Client{},
			maxRange: int64(blobSize),
		}

		for i := 0; i < numberOfBlobs; i++ {
			blobUpload.location = e + locationPath
			n, err := blobUpload.Write(buff)
			if err != nil {
				t.Fatalf("Error calling Write: %s", err)
			}

			if n != blobSize {
				t.Fatalf("Unexpected n %v != %v blobSize", n, blobSize)
			}

			buff = buff[n:]
		}

		n, err := blobUpload.Write(buff)
		if err != nil {
			t.Fatalf("Error calling Write: %s", err)
		}

		if n != lastBlobOffset {
			t.Fatalf("Expected last write to have written %v but wrote %v", lastBlobOffset, n)
		}

		buff = buff[n:]
		if len(buff) != 0 {
			t.Fatalf("Expected length 0 on the buffer body as we should've read the whole thing, but got %v", len(buff))
		}
	})
}
