// +build ignore

package ipc

import (
	"fmt"
	"io"
	"reflect"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/libchan"
)

// StorageDriver is the interface which IPC storage drivers must implement. As external storage
// drivers may be defined to use a different version of the storagedriver.StorageDriver interface,
// we use an additional version check to determine compatiblity.
type StorageDriver interface {
	// Version returns the storagedriver.StorageDriver interface version which this storage driver
	// implements, which is used to determine driver compatibility
	Version() (storagedriver.Version, error)
}

// IncompatibleVersionError is returned when a storage driver is using an incompatible version of
// the storagedriver.StorageDriver api
type IncompatibleVersionError struct {
	version storagedriver.Version
}

func (e IncompatibleVersionError) Error() string {
	return fmt.Sprintf("Incompatible storage driver version: %s", e.version)
}

// Request defines a remote method call request
// A return value struct is to be sent over the ResponseChannel
type Request struct {
	Type            string                 `codec:",omitempty"`
	Parameters      map[string]interface{} `codec:",omitempty"`
	ResponseChannel libchan.Sender         `codec:",omitempty"`
}

// ResponseError is a serializable error type.
// The Type and Parameters may be used to reconstruct the same error on the
// client side, falling back to using the Type and Message if this cannot be
// done.
type ResponseError struct {
	Type       string                 `codec:",omitempty"`
	Message    string                 `codec:",omitempty"`
	Parameters map[string]interface{} `codec:",omitempty"`
}

// WrapError wraps an error in a serializable struct containing the error's type
// and message.
func WrapError(err error) *ResponseError {
	if err == nil {
		return nil
	}
	v := reflect.ValueOf(err)
	re := ResponseError{
		Type:    v.Type().String(),
		Message: err.Error(),
	}

	if v.Kind() == reflect.Struct {
		re.Parameters = make(map[string]interface{})
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			re.Parameters[field.Name] = v.Field(i).Interface()
		}
	}
	return &re
}

// Unwrap returns the underlying error if it can be reconstructed, or the
// original ResponseError otherwise.
func (err *ResponseError) Unwrap() error {
	var errVal reflect.Value
	var zeroVal reflect.Value

	switch err.Type {
	case "storagedriver.PathNotFoundError":
		errVal = reflect.ValueOf(&storagedriver.PathNotFoundError{})
	case "storagedriver.InvalidOffsetError":
		errVal = reflect.ValueOf(&storagedriver.InvalidOffsetError{})
	}
	if errVal == zeroVal {
		return err
	}

	for k, v := range err.Parameters {
		fieldVal := errVal.Elem().FieldByName(k)
		if fieldVal == zeroVal {
			return err
		}
		fieldVal.Set(reflect.ValueOf(v))
	}

	if unwrapped, ok := errVal.Elem().Interface().(error); ok {
		return unwrapped
	}

	return err

}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("%s: %s", err.Type, err.Message)
}

// IPC method call response object definitions

// VersionResponse is a response for a Version request
type VersionResponse struct {
	Version storagedriver.Version `codec:",omitempty"`
	Error   *ResponseError        `codec:",omitempty"`
}

// ReadStreamResponse is a response for a ReadStream request
type ReadStreamResponse struct {
	Reader io.ReadCloser  `codec:",omitempty"`
	Error  *ResponseError `codec:",omitempty"`
}

// WriteStreamResponse is a response for a WriteStream request
type WriteStreamResponse struct {
	Error *ResponseError `codec:",omitempty"`
}

// CurrentSizeResponse is a response for a CurrentSize request
type CurrentSizeResponse struct {
	Position uint64         `codec:",omitempty"`
	Error    *ResponseError `codec:",omitempty"`
}

// ListResponse is a response for a List request
type ListResponse struct {
	Keys  []string       `codec:",omitempty"`
	Error *ResponseError `codec:",omitempty"`
}

// MoveResponse is a response for a Move request
type MoveResponse struct {
	Error *ResponseError `codec:",omitempty"`
}

// DeleteResponse is a response for a Delete request
type DeleteResponse struct {
	Error *ResponseError `codec:",omitempty"`
}
