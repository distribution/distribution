package ipc

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

	"github.com/docker/libchan"
)

// Request defines a remote method call request
// A return value struct is to be sent over the ResponseChannel
type Request struct {
	Type            string
	Parameters      map[string]interface{}
	ResponseChannel libchan.Sender
}

// noWriteReadWriteCloser is a simple wrapper around an io.ReadCloser that implements the
// io.ReadWriteCloser interface
// Calls to Write are disallowed and will return an error
type noWriteReadWriteCloser struct {
	io.ReadCloser
}

func (r noWriteReadWriteCloser) Write(p []byte) (n int, err error) {
	return 0, errors.New("Write unsupported")
}

// WrapReader wraps an io.Reader as an io.ReadWriteCloser with a nop Close and unsupported Write
// Has no effect when an io.ReadWriteCloser is passed in
func WrapReader(reader io.Reader) io.ReadWriteCloser {
	if readWriteCloser, ok := reader.(io.ReadWriteCloser); ok {
		return readWriteCloser
	} else if readCloser, ok := reader.(io.ReadCloser); ok {
		return noWriteReadWriteCloser{readCloser}
	} else {
		return noWriteReadWriteCloser{ioutil.NopCloser(reader)}
	}
}

type responseError struct {
	Type    string
	Message string
}

// ResponseError wraps an error in a serializable struct containing the error's type and message
func ResponseError(err error) *responseError {
	if err == nil {
		return nil
	}
	return &responseError{
		Type:    reflect.TypeOf(err).String(),
		Message: err.Error(),
	}
}

func (err *responseError) Error() string {
	return fmt.Sprintf("%s: %s", err.Type, err.Message)
}

// IPC method call response object definitions

// ReadStreamResponse is a response for a ReadStream request
type ReadStreamResponse struct {
	Reader io.ReadWriteCloser
	Error  *responseError
}

// WriteStreamResponse is a response for a WriteStream request
type WriteStreamResponse struct {
	Error *responseError
}

// ResumeWritePositionResponse is a response for a ResumeWritePosition request
type ResumeWritePositionResponse struct {
	Position uint64
	Error    *responseError
}

// ListResponse is a response for a List request
type ListResponse struct {
	Keys  []string
	Error *responseError
}

// MoveResponse is a response for a Move request
type MoveResponse struct {
	Error *responseError
}

// DeleteResponse is a response for a Delete request
type DeleteResponse struct {
	Error *responseError
}
