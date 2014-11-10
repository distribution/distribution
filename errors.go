package registry

import (
	"fmt"
	"strings"
)

// ErrorCode represents the error type. The errors are serialized via strings
// and the integer format may change and should *never* be exported.
type ErrorCode int

const (
	ErrorCodeUnknown ErrorCode = iota

	// The following errors can happen during a layer upload.
	ErrorCodeInvalidChecksum
	ErrorCodeInvalidLength
	ErrorCodeInvalidTarsum

	// The following errors can happen during manifest upload.
	ErrorCodeInvalidName
	ErrorCodeInvalidTag
	ErrorCodeUnverifiedManifest
	ErrorCodeUnknownLayer
	ErrorCodeUntrustedSignature
)

var errorCodeStrings = map[ErrorCode]string{
	ErrorCodeUnknown:            "UNKNOWN",
	ErrorCodeInvalidChecksum:    "INVALID_CHECKSUM",
	ErrorCodeInvalidLength:      "INVALID_LENGTH",
	ErrorCodeInvalidTarsum:      "INVALID_TARSUM",
	ErrorCodeInvalidName:        "INVALID_NAME",
	ErrorCodeInvalidTag:         "INVALID_TAG",
	ErrorCodeUnverifiedManifest: "UNVERIFIED_MANIFEST",
	ErrorCodeUnknownLayer:       "UNKNOWN_LAYER",
	ErrorCodeUntrustedSignature: "UNTRUSTED_SIGNATURE",
}

var errorCodesMessages = map[ErrorCode]string{
	ErrorCodeUnknown:            "unknown error",
	ErrorCodeInvalidChecksum:    "provided checksum did not match uploaded content",
	ErrorCodeInvalidLength:      "provided length did not match content length",
	ErrorCodeInvalidTarsum:      "provided tarsum did not match binary content",
	ErrorCodeInvalidName:        "Manifest name did not match URI",
	ErrorCodeInvalidTag:         "Manifest tag did not match URI",
	ErrorCodeUnverifiedManifest: "Manifest failed signature validation",
	ErrorCodeUnknownLayer:       "Referenced layer not available",
	ErrorCodeUntrustedSignature: "Manifest signed by untrusted source",
}

var stringToErrorCode map[string]ErrorCode

func init() {
	stringToErrorCode = make(map[string]ErrorCode, len(errorCodeStrings))

	// Build up reverse error code map
	for k, v := range errorCodeStrings {
		stringToErrorCode[v] = k
	}
}

// ParseErrorCode attempts to parse the error code string, returning
// ErrorCodeUnknown if the error is not known.
func ParseErrorCode(s string) ErrorCode {
	ec, ok := stringToErrorCode[s]

	if !ok {
		return ErrorCodeUnknown
	}

	return ec
}

// String returns the canonical identifier for this error code.
func (ec ErrorCode) String() string {
	s, ok := errorCodeStrings[ec]

	if !ok {
		return errorCodeStrings[ErrorCodeUnknown]
	}

	return s
}

func (ec ErrorCode) Message() string {
	m, ok := errorCodesMessages[ec]

	if !ok {
		return errorCodesMessages[ErrorCodeUnknown]
	}

	return m
}

func (ec ErrorCode) MarshalText() (text []byte, err error) {
	return []byte(ec.String()), nil
}

func (ec *ErrorCode) UnmarshalText(text []byte) error {
	*ec = stringToErrorCode[string(text)]

	return nil
}

type Error struct {
	Code    ErrorCode   `json:"code,omitempty"`
	Message string      `json:"message,omitempty"`
	Detail  interface{} `json:"detail,omitempty"`
}

// Error returns a human readable representation of the error.
func (e Error) Error() string {
	return fmt.Sprintf("%s: %s",
		strings.Title(strings.Replace(e.Code.String(), "_", " ", -1)),
		e.Message)
}

// Errors provides the envelope for multiple errors and a few sugar methods
// for use within the application.
type Errors struct {
	Errors []Error `json:"errors,omitempty"`
}

// Push pushes an error on to the error stack, with the optional detail
// argument. It is a programming error (ie panic) to push more than one
// detail at a time.
func (errs *Errors) Push(code ErrorCode, details ...interface{}) {
	if len(details) > 1 {
		panic("please specify zero or one detail items for this error")
	}

	var detail interface{}
	if len(details) > 0 {
		detail = details[0]
	}

	errs.Errors = append(errs.Errors, Error{
		Code:    code,
		Message: code.Message(),
		Detail:  detail,
	})
}

// detailUnknownLayer provides detail for unknown layer errors, returned by
// image manifest push for layers that are not yet transferred. This intended
// to only be used on the backend to return detail for this specific error.
type DetailUnknownLayer struct {

	// Unknown should contain the contents of a layer descriptor, which is a
	// single json object with the key "blobSum" currently.
	Unknown struct {

		// BlobSum contains the uniquely identifying tarsum of the layer.
		BlobSum string `json:"blobSum"`
	} `json:"unknown"`
}
