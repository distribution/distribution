package ipc

import (
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/docker/libchan"
)

type Request struct {
	Type            string
	Parameters      map[string]interface{}
	ResponseChannel libchan.Sender
}

type noWriteReadWriteCloser struct {
	io.ReadCloser
}

func (r noWriteReadWriteCloser) Write(p []byte) (n int, err error) {
	return 0, errors.New("Write unsupported")
}

func WrapReadCloser(readCloser io.ReadCloser) io.ReadWriteCloser {
	return noWriteReadWriteCloser{readCloser}
}

type responseError struct {
	Type    string
	Message string
}

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

type GetContentResponse struct {
	Content []byte
	Error   *responseError
}

type PutContentResponse struct {
	Error *responseError
}

type ReadStreamResponse struct {
	Reader io.ReadWriteCloser
	Error  *responseError
}

type WriteStreamResponse struct {
	Error *responseError
}

type ResumeWritePositionResponse struct {
	Position uint64
	Error    *responseError
}

type ListResponse struct {
	Keys  []string
	Error *responseError
}

type MoveResponse struct {
	Error *responseError
}

type DeleteResponse struct {
	Error *responseError
}
