package errors

import "net/http"

// ErrorDescriptor provides relevant information about a given error code.
type ErrorDescriptor struct {
	// Code is the error code that this descriptor describes.
	Code ErrorCode

	// Value provides a unique, string key, often captilized with
	// underscores, to identify the error code. This value is used as the
	// keyed value when serializing api errors.
	Value string

	// Message is a short, human readable decription of the error condition
	// included in API responses.
	Message string

	// Description provides a complete account of the errors purpose, suitable
	// for use in documentation.
	Description string

	// DefaultStatusCode should to be returned via the HTTP API. Some error
	// may have different status codes depending on the situation.
	DefaultStatusCode int
}

var descriptors = []ErrorDescriptor{
	{
		Code:    ErrorCodeUnknown,
		Value:   "UNKNOWN",
		Message: "unknown error",
	},
	{
		Code:    ErrorCodeDigestInvalid,
		Value:   "DIGEST_INVALID",
		Message: "provided digest did not match uploaded content",
		Description: `When a blob is uploaded, the registry will check that
		the content matches the digest provided by the client. The error may
		include a detail structure with the key "digest", including the
		invalid digest string. This error may also be returned when a manifest
		includes an invalid layer digest.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeSizeInvalid,
		Value:   "SIZE_INVALID",
		Message: "provided length did not match content length",
		Description: `When a layer is uploaded, the provided size will be
		checked against the uploaded content. If they do not match, this error
		will be returned.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeNameInvalid,
		Value:   "NAME_INVALID",
		Message: "manifest name did not match URI",
		Description: `During a manifest upload, if the name in the manifest
		does not match the uri name, this error will be returned.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeTagInvalid,
		Value:   "TAG_INVALID",
		Message: "manifest tag did not match URI",
		Description: `During a manifest upload, if the tag in the manifest
		does not match the uri tag, this error will be returned.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeNameUnknown,
		Value:   "NAME_UNKNOWN",
		Message: "repository name not known to registry",
		Description: `This is returned if the name used during an operation is
		unknown to the registry.`,
		DefaultStatusCode: http.StatusNotFound,
	},
	{
		Code:    ErrorCodeManifestUnknown,
		Value:   "MANIFEST_UNKNOWN",
		Message: "manifest unknown",
		Description: `This error is returned when the manifest, identified by
		name and tag is unknown to the repository.`,
		DefaultStatusCode: http.StatusNotFound,
	},
	{
		Code:    ErrorCodeManifestInvalid,
		Value:   "MANIFEST_INVALID",
		Message: "manifest invalid",
		Description: `During upload, manifests undergo several checks ensuring
		validity. If those checks fail, this error may be returned, unless a
		more specific error is included.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeManifestUnverified,
		Value:   "MANIFEST_UNVERIFIED",
		Message: "manifest failed signature verification",
		Description: `During manifest upload, if the manifest fails signature
		verification, this error will be returned.`,
		DefaultStatusCode: http.StatusBadRequest,
	},
	{
		Code:    ErrorCodeBlobUnknown,
		Value:   "BLOB_UNKNOWN",
		Message: "blob unknown to registry",
		Description: `This error may be returned when a blob is unknown to the
		registry in a specified repository. This can be returned with a
		standard get or if a manifest references an unknown layer during
		upload.`,
		DefaultStatusCode: http.StatusNotFound,
	},

	{
		Code:    ErrorCodeBlobUploadUnknown,
		Value:   "BLOB_UPLOAD_UNKNOWN",
		Message: "blob upload unknown to registry",
		Description: `If a blob upload has been cancelled or was never
		started, this error code may be returned.`,
		DefaultStatusCode: http.StatusNotFound,
	},
}

var errorCodeToDescriptors map[ErrorCode]ErrorDescriptor
var idToDescriptors map[string]ErrorDescriptor

func init() {
	errorCodeToDescriptors = make(map[ErrorCode]ErrorDescriptor, len(descriptors))
	idToDescriptors = make(map[string]ErrorDescriptor, len(descriptors))

	for _, descriptor := range descriptors {
		errorCodeToDescriptors[descriptor.Code] = descriptor
		idToDescriptors[descriptor.Value] = descriptor
	}
}
