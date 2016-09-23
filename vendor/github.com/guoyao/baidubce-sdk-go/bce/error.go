package bce

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Error implements the error interface
type Error struct {
	StatusCode               int
	Code, Message, RequestID string
}

func (err *Error) Error() string {
	return fmt.Sprintf("Error Message: \"%s\", Error Code: \"%s\", Status Code: %d, Request Id: \"%s\"",
		err.Message, err.Code, err.StatusCode, err.RequestID)
}

func buildError(resp *Response) error {
	bodyContent, err := resp.GetBodyContent()

	if err == nil {
		if bodyContent == nil || string(bodyContent) == "" {
			return errors.New("Unknown Error")
		}

		var bceError *Error
		err := json.Unmarshal(bodyContent, &bceError)

		if err != nil {
			return errors.New(string(bodyContent))
		}

		bceError.StatusCode = resp.StatusCode

		return bceError
	}

	return err
}
