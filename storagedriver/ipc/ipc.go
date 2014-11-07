package ipc

import (
	"fmt"
	"io"
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
	Reader io.ReadCloser
	Error  *responseError
}

// WriteStreamResponse is a response for a WriteStream request
type WriteStreamResponse struct {
	Error *responseError
}

// CurrentSizeResponse is a response for a CurrentSize request
type CurrentSizeResponse struct {
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
