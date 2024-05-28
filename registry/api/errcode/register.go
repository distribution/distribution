package errcode

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

var (
	errorCodeToDescriptors = map[ErrorCode]ErrorDescriptor{}
	idToDescriptors        = map[string]ErrorDescriptor{}
	groupToDescriptors     = map[string][]ErrorDescriptor{}
)

var (
	// ErrorCodeUnknown is a generic error that can be used as a last
	// resort if there is no situation-specific error message that can be used
	ErrorCodeUnknown = register("errcode", ErrorDescriptor{
		Value:   "UNKNOWN",
		Message: "unknown error",
		Description: `Generic error returned when the error does not have an
			                                            API classification.`,
		HTTPStatusCode: http.StatusInternalServerError,
	})

	// ErrorCodeUnsupported is returned when an operation is not supported.
	ErrorCodeUnsupported = register("errcode", ErrorDescriptor{
		Value:   "UNSUPPORTED",
		Message: "The operation is unsupported.",
		Description: `The operation was unsupported due to a missing
		implementation or invalid set of parameters.`,
		HTTPStatusCode: http.StatusMethodNotAllowed,
	})

	// ErrorCodeUnauthorized is returned if a request requires
	// authentication.
	ErrorCodeUnauthorized = register("errcode", ErrorDescriptor{
		Value:   "UNAUTHORIZED",
		Message: "authentication required",
		Description: `The access controller was unable to authenticate
		the client. Often this will be accompanied by a
		Www-Authenticate HTTP response header indicating how to
		authenticate.`,
		HTTPStatusCode: http.StatusUnauthorized,
	})

	// ErrorCodeDenied is returned if a client does not have sufficient
	// permission to perform an action.
	ErrorCodeDenied = register("errcode", ErrorDescriptor{
		Value:   "DENIED",
		Message: "requested access to the resource is denied",
		Description: `The access controller denied access for the
		operation on a resource.`,
		HTTPStatusCode: http.StatusForbidden,
	})

	// ErrorCodeUnavailable provides a common error to report unavailability
	// of a service or endpoint.
	ErrorCodeUnavailable = register("errcode", ErrorDescriptor{
		Value:          "UNAVAILABLE",
		Message:        "service unavailable",
		Description:    "Returned when a service is not available",
		HTTPStatusCode: http.StatusServiceUnavailable,
	})

	// ErrorCodeTooManyRequests is returned if a client attempts too many
	// times to contact a service endpoint.
	ErrorCodeTooManyRequests = register("errcode", ErrorDescriptor{
		Value:   "TOOMANYREQUESTS",
		Message: "too many requests",
		Description: `Returned when a client attempts to contact a
		service too many times`,
		HTTPStatusCode: http.StatusTooManyRequests,
	})
)

const errGroup = "registry.api.v2"

var (
	// ErrorCodeDigestInvalid is returned when uploading a blob if the
	// provided digest does not match the blob contents.
	ErrorCodeDigestInvalid = register(errGroup, ErrorDescriptor{
		Value:   "DIGEST_INVALID",
		Message: "provided digest did not match uploaded content",
		Description: `When a blob is uploaded, the registry will check that
		the content matches the digest provided by the client. The error may
		include a detail structure with the key "digest", including the
		invalid digest string. This error may also be returned when a manifest
		includes an invalid layer digest.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeSizeInvalid is returned when uploading a blob if the provided
	ErrorCodeSizeInvalid = register(errGroup, ErrorDescriptor{
		Value:   "SIZE_INVALID",
		Message: "provided length did not match content length",
		Description: `When a layer is uploaded, the provided size will be
		checked against the uploaded content. If they do not match, this error
		will be returned.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeRangeInvalid is returned when uploading a blob if the provided
	// content range is invalid.
	ErrorCodeRangeInvalid = register(errGroup, ErrorDescriptor{
		Value:   "RANGE_INVALID",
		Message: "invalid content range",
		Description: `When a layer is uploaded, the provided range is checked
		against the uploaded chunk. This error is returned if the range is
		out of order.`,
		HTTPStatusCode: http.StatusRequestedRangeNotSatisfiable,
	})

	// ErrorCodeNameInvalid is returned when the name in the manifest does not
	// match the provided name.
	ErrorCodeNameInvalid = register(errGroup, ErrorDescriptor{
		Value:   "NAME_INVALID",
		Message: "invalid repository name",
		Description: `Invalid repository name encountered either during
		manifest validation or any API operation.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeTagInvalid is returned when the tag in the manifest does not
	// match the provided tag.
	ErrorCodeTagInvalid = register(errGroup, ErrorDescriptor{
		Value:   "TAG_INVALID",
		Message: "manifest tag did not match URI",
		Description: `During a manifest upload, if the tag in the manifest
		does not match the uri tag, this error will be returned.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeNameUnknown when the repository name is not known.
	ErrorCodeNameUnknown = register(errGroup, ErrorDescriptor{
		Value:   "NAME_UNKNOWN",
		Message: "repository name not known to registry",
		Description: `This is returned if the name used during an operation is
		unknown to the registry.`,
		HTTPStatusCode: http.StatusNotFound,
	})

	// ErrorCodeManifestUnknown returned when image manifest is unknown.
	ErrorCodeManifestUnknown = register(errGroup, ErrorDescriptor{
		Value:   "MANIFEST_UNKNOWN",
		Message: "manifest unknown",
		Description: `This error is returned when the manifest, identified by
		name and tag is unknown to the repository.`,
		HTTPStatusCode: http.StatusNotFound,
	})

	// ErrorCodeManifestInvalid returned when an image manifest is invalid,
	// typically during a PUT operation. This error encompasses all errors
	// encountered during manifest validation that aren't signature errors.
	ErrorCodeManifestInvalid = register(errGroup, ErrorDescriptor{
		Value:   "MANIFEST_INVALID",
		Message: "manifest invalid",
		Description: `During upload, manifests undergo several checks ensuring
		validity. If those checks fail, this error may be returned, unless a
		more specific error is included. The detail will contain information
		the failed validation.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeManifestUnverified is returned when the manifest fails
	// signature verification.
	ErrorCodeManifestUnverified = register(errGroup, ErrorDescriptor{
		Value:   "MANIFEST_UNVERIFIED",
		Message: "manifest failed signature verification",
		Description: `During manifest upload, if the manifest fails signature
		verification, this error will be returned.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeManifestBlobUnknown is returned when a manifest blob is
	// unknown to the registry.
	ErrorCodeManifestBlobUnknown = register(errGroup, ErrorDescriptor{
		Value:   "MANIFEST_BLOB_UNKNOWN",
		Message: "blob unknown to registry",
		Description: `This error may be returned when a manifest blob is 
		unknown to the registry.`,
		HTTPStatusCode: http.StatusBadRequest,
	})

	// ErrorCodeBlobUnknown is returned when a blob is unknown to the
	// registry. This can happen when the manifest references a nonexistent
	// layer or the result is not found by a blob fetch.
	ErrorCodeBlobUnknown = register(errGroup, ErrorDescriptor{
		Value:   "BLOB_UNKNOWN",
		Message: "blob unknown to registry",
		Description: `This error may be returned when a blob is unknown to the
		registry in a specified repository. This can be returned with a
		standard get or if a manifest references an unknown layer during
		upload.`,
		HTTPStatusCode: http.StatusNotFound,
	})

	// ErrorCodeBlobUploadUnknown is returned when an upload is unknown.
	ErrorCodeBlobUploadUnknown = register(errGroup, ErrorDescriptor{
		Value:   "BLOB_UPLOAD_UNKNOWN",
		Message: "blob upload unknown to registry",
		Description: `If a blob upload has been cancelled or was never
		started, this error code may be returned.`,
		HTTPStatusCode: http.StatusNotFound,
	})

	// ErrorCodeBlobUploadInvalid is returned when an upload is invalid.
	ErrorCodeBlobUploadInvalid = register(errGroup, ErrorDescriptor{
		Value:   "BLOB_UPLOAD_INVALID",
		Message: "blob upload invalid",
		Description: `The blob upload encountered an error and can no
		longer proceed.`,
		HTTPStatusCode: http.StatusNotFound,
	})

	// ErrorCodePaginationNumberInvalid is returned when the `n` parameter is
	// not an integer, or `n` is negative.
	ErrorCodePaginationNumberInvalid = register(errGroup, ErrorDescriptor{
		Value:   "PAGINATION_NUMBER_INVALID",
		Message: "invalid number of results requested",
		Description: `Returned when the "n" parameter (number of results
		to return) is not an integer, "n" is negative or "n" is bigger than
		the maximum allowed.`,
		HTTPStatusCode: http.StatusBadRequest,
	})
)

var (
	nextCode     = 1000
	registerLock sync.Mutex
)

// Register will make the passed-in error known to the environment and
// return a new ErrorCode
func Register(group string, descriptor ErrorDescriptor) ErrorCode {
	return register(group, descriptor)
}

// register will make the passed-in error known to the environment and
// return a new ErrorCode
func register(group string, descriptor ErrorDescriptor) ErrorCode {
	registerLock.Lock()
	defer registerLock.Unlock()

	descriptor.Code = ErrorCode(nextCode)

	if _, ok := idToDescriptors[descriptor.Value]; ok {
		panic(fmt.Sprintf("ErrorValue %q is already registered", descriptor.Value))
	}
	if _, ok := errorCodeToDescriptors[descriptor.Code]; ok {
		panic(fmt.Sprintf("ErrorCode %v is already registered", descriptor.Code))
	}

	groupToDescriptors[group] = append(groupToDescriptors[group], descriptor)
	errorCodeToDescriptors[descriptor.Code] = descriptor
	idToDescriptors[descriptor.Value] = descriptor

	nextCode++
	return descriptor.Code
}

type byValue []ErrorDescriptor

func (a byValue) Len() int           { return len(a) }
func (a byValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byValue) Less(i, j int) bool { return a[i].Value < a[j].Value }

// GetGroupNames returns the list of Error group names that are registered
func GetGroupNames() []string {
	keys := []string{}

	for k := range groupToDescriptors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetErrorCodeGroup returns the named group of error descriptors
func GetErrorCodeGroup(name string) []ErrorDescriptor {
	desc := groupToDescriptors[name]
	sort.Sort(byValue(desc))
	return desc
}

// GetErrorAllDescriptors returns a slice of all ErrorDescriptors that are
// registered, irrespective of what group they're in
func GetErrorAllDescriptors() []ErrorDescriptor {
	result := []ErrorDescriptor{}

	for _, group := range GetGroupNames() {
		result = append(result, GetErrorCodeGroup(group)...)
	}
	sort.Sort(byValue(result))
	return result
}
