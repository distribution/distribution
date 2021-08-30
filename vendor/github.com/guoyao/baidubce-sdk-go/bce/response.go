package bce

import (
	"io/ioutil"
	"net/http"
)

// Response holds an instance of type `http response`, and has some custom data and functions.
type Response struct {
	BodyContent []byte
	*http.Response
}

func NewResponse(res *http.Response) *Response {
	return &Response{Response: res}
}

// GetBodyContent gets body from http response.
func (res *Response) GetBodyContent() ([]byte, error) {
	if res.BodyContent == nil {
		defer func() {
			if res.Response != nil {
				res.Body.Close()
			}
		}()

		body, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return nil, err
		}

		res.BodyContent = body
	}

	return res.BodyContent, nil
}
