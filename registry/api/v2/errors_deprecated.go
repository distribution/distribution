package v2

import "github.com/distribution/distribution/v3/registry/api/errcode"

var (
	// ErrorCodeDigestInvalid is returned when uploading a blob if the
	// provided digest does not match the blob contents.
	//
	// Deprecated: use [errcode.ErrorCodeDigestInvalid].
	ErrorCodeDigestInvalid = errcode.ErrorCodeDigestInvalid

	// ErrorCodeSizeInvalid is returned when uploading a blob if the provided
	//
	// Deprecated: use [errcode.ErrorCodeSizeInvalid].
	ErrorCodeSizeInvalid = errcode.ErrorCodeSizeInvalid

	// ErrorCodeRangeInvalid is returned when uploading a blob if the provided
	// content range is invalid.
	//
	// Deprecated: use [errcode.ErrorCodeRangeInvalid].
	ErrorCodeRangeInvalid = errcode.ErrorCodeRangeInvalid

	// ErrorCodeNameInvalid is returned when the name in the manifest does not
	// match the provided name.
	//
	// Deprecated: use [errcode.ErrorCodeNameInvalid].
	ErrorCodeNameInvalid = errcode.ErrorCodeNameInvalid

	// ErrorCodeTagInvalid is returned when the tag in the manifest does not
	// match the provided tag.
	//
	// Deprecated: use [errcode.ErrorCodeTagInvalid].
	ErrorCodeTagInvalid = errcode.ErrorCodeTagInvalid

	// ErrorCodeNameUnknown when the repository name is not known.
	//
	// Deprecated: use [errcode.ErrorCodeNameUnknown].
	ErrorCodeNameUnknown = errcode.ErrorCodeNameUnknown

	// ErrorCodeManifestUnknown returned when image manifest is unknown.
	//
	// Deprecated: use [errcode.ErrorCodeManifestUnknown].
	ErrorCodeManifestUnknown = errcode.ErrorCodeManifestUnknown

	// ErrorCodeManifestInvalid returned when an image manifest is invalid,
	// typically during a PUT operation. This error encompasses all errors
	// encountered during manifest validation that aren't signature errors.
	//
	// Deprecated: use [errcode.ErrorCodeManifestInvalid].
	ErrorCodeManifestInvalid = errcode.ErrorCodeManifestInvalid

	// ErrorCodeManifestUnverified is returned when the manifest fails
	// signature verification.
	//
	// Deprecated: use [errcode.ErrorCodeManifestUnverified].
	ErrorCodeManifestUnverified = errcode.ErrorCodeManifestUnverified

	// ErrorCodeManifestBlobUnknown is returned when a manifest blob is
	// unknown to the registry.
	//
	// Deprecated: use [errcode.ErrorCodeManifestBlobUnknown].
	ErrorCodeManifestBlobUnknown = errcode.ErrorCodeManifestBlobUnknown

	// ErrorCodeBlobUnknown is returned when a blob is unknown to the
	// registry. This can happen when the manifest references a nonexistent
	// layer or the result is not found by a blob fetch.
	//
	// Deprecated: use [errcode.ErrorCodeBlobUnknown].
	ErrorCodeBlobUnknown = errcode.ErrorCodeBlobUnknown

	// ErrorCodeBlobUploadUnknown is returned when an upload is unknown.
	//
	// Deprecated: use [errcode.ErrorCodeBlobUploadUnknown].
	ErrorCodeBlobUploadUnknown = errcode.ErrorCodeBlobUploadUnknown

	// ErrorCodeBlobUploadInvalid is returned when an upload is invalid.
	//
	// Deprecated: use [errcode.ErrorCodeBlobUploadInvalid].
	ErrorCodeBlobUploadInvalid = errcode.ErrorCodeBlobUploadInvalid

	// ErrorCodePaginationNumberInvalid is returned when the `n` parameter is
	// not an integer, or `n` is negative.
	//
	// Deprecated: use [errcode.ErrorCodePaginationNumberInvalid].
	ErrorCodePaginationNumberInvalid = errcode.ErrorCodePaginationNumberInvalid
)
