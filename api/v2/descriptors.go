package v2

import (
	"net/http"
	"regexp"

	"github.com/docker/distribution/digest"
)

var (
	nameParameterDescriptor = ParameterDescriptor{
		Name:        "name",
		Type:        "string",
		Format:      RepositoryNameRegexp.String(),
		Required:    true,
		Description: `Name of the target repository.`,
	}

	tagParameterDescriptor = ParameterDescriptor{
		Name:        "tag",
		Type:        "string",
		Format:      TagNameRegexp.String(),
		Required:    true,
		Description: `Tag of the target manifiest.`,
	}

	uuidParameterDescriptor = ParameterDescriptor{
		Name:        "uuid",
		Type:        "opaque",
		Required:    true,
		Description: `A uuid identifying the upload. This field can accept almost anything.`,
	}

	digestPathParameter = ParameterDescriptor{
		Name:        "digest",
		Type:        "path",
		Required:    true,
		Format:      digest.DigestRegexp.String(),
		Description: `Digest of desired blob.`,
	}

	authHeader = ParameterDescriptor{
		Name:        "Authorization",
		Type:        "string",
		Description: "rfc7235 compliant authorization header.",
		Format:      "<scheme> <token>",
		Examples:    []string{"Bearer dGhpcyBpcyBhIGZha2UgYmVhcmVyIHRva2VuIQ=="},
	}

	authChallengeHeader = ParameterDescriptor{
		Name:        "WWW-Authenticate",
		Type:        "string",
		Description: "An RFC7235 compliant authentication challenge header.",
		Format:      `<scheme> realm="<realm>", ..."`,
		Examples: []string{
			`Bearer realm="https://auth.docker.com/", service="registry.docker.com", scopes="repository:library/ubuntu:pull"`,
		},
	}

	contentLengthZeroHeader = ParameterDescriptor{
		Name:        "Content-Length",
		Description: "The `Content-Length` header must be zero and the body must be empty.",
		Type:        "integer",
		Format:      "0",
	}
)

const (
	manifestBody = `{
   "name": <name>,
   "tag": <tag>,
   "fsLayers": [
      {
         "blobSum": <tarsum>
      },
      ...
    ]
   ],
   "history": <v1 images>,
   "signature": <JWS>
}`

	errorsBody = `{
	"errors:" [{
            "code": <error code>,
            "message": "<error message>",
            "detail": ...
        },
        ...
    ]
}`
)

// APIDescriptor exports descriptions of the layout of the v2 registry API.
var APIDescriptor = struct {
	// RouteDescriptors provides a list of the routes available in the API.
	RouteDescriptors []RouteDescriptor

	// ErrorDescriptors provides a list of the error codes and their
	// associated documentation and metadata.
	ErrorDescriptors []ErrorDescriptor
}{
	RouteDescriptors: routeDescriptors,
	ErrorDescriptors: errorDescriptors,
}

// RouteDescriptor describes a route specified by name.
type RouteDescriptor struct {
	// Name is the name of the route, as specified in RouteNameXXX exports.
	// These names a should be considered a unique reference for a route. If
	// the route is registered with gorilla, this is the name that will be
	// used.
	Name string

	// Path is a gorilla/mux-compatible regexp that can be used to match the
	// route. For any incoming method and path, only one route descriptor
	// should match.
	Path string

	// Entity should be a short, human-readalbe description of the object
	// targeted by the endpoint.
	Entity string

	// Description should provide an accurate overview of the functionality
	// provided by the route.
	Description string

	// Methods should describe the various HTTP methods that may be used on
	// this route, including request and response formats.
	Methods []MethodDescriptor
}

// MethodDescriptor provides a description of the requests that may be
// conducted with the target method.
type MethodDescriptor struct {

	// Method is an HTTP method, such as GET, PUT or POST.
	Method string

	// Description should provide an overview of the functionality provided by
	// the covered method, suitable for use in documentation. Use of markdown
	// here is encouraged.
	Description string

	// Requests is a slice of request descriptors enumerating how this
	// endpoint may be used.
	Requests []RequestDescriptor
}

// RequestDescriptor covers a particular set of headers and parameters that
// can be carried out with the parent method. Its most helpful to have one
// RequestDescriptor per API use case.
type RequestDescriptor struct {
	// Name provides a short identifier for the request, usable as a title or
	// to provide quick context for the particalar request.
	Name string

	// Description should cover the requests purpose, covering any details for
	// this particular use case.
	Description string

	// Headers describes headers that must be used with the HTTP request.
	Headers []ParameterDescriptor

	// PathParameters enumerate the parameterized path components for the
	// given request, as defined in the route's regular expression.
	PathParameters []ParameterDescriptor

	// QueryParameters provides a list of query parameters for the given
	// request.
	QueryParameters []ParameterDescriptor

	// Body describes the format of the request body.
	Body BodyDescriptor

	// Successes enumerates the possible responses that are considered to be
	// the result of a successful request.
	Successes []ResponseDescriptor

	// Failures covers the possible failures from this particular request.
	Failures []ResponseDescriptor
}

// ResponseDescriptor describes the components of an API response.
type ResponseDescriptor struct {
	// Name provides a short identifier for the response, usable as a title or
	// to provide quick context for the particalar response.
	Name string

	// Description should provide a brief overview of the role of the
	// response.
	Description string

	// StatusCode specifies the status recieved by this particular response.
	StatusCode int

	// Headers covers any headers that may be returned from the response.
	Headers []ParameterDescriptor

	// ErrorCodes enumerates the error codes that may be returned along with
	// the response.
	ErrorCodes []ErrorCode

	// Body describes the body of the response, if any.
	Body BodyDescriptor
}

// BodyDescriptor describes a request body and its expected content type. For
// the most  part, it should be example json or some placeholder for body
// data in documentation.
type BodyDescriptor struct {
	ContentType string
	Format      string
}

// ParameterDescriptor describes the format of a request parameter, which may
// be a header, path parameter or query parameter.
type ParameterDescriptor struct {
	// Name is the name of the parameter, either of the path component or
	// query parameter.
	Name string

	// Type specifies the type of the parameter, such as string, integer, etc.
	Type string

	// Description provides a human-readable description of the parameter.
	Description string

	// Required means the field is required when set.
	Required bool

	// Format is a specifying the string format accepted by this parameter.
	Format string

	// Regexp is a compiled regular expression that can be used to validate
	// the contents of the parameter.
	Regexp *regexp.Regexp

	// Examples provides multiple examples for the values that might be valid
	// for this parameter.
	Examples []string
}

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

	// HTTPStatusCodes provides a list of status under which this error
	// condition may arise. If it is empty, the error condition may be seen
	// for any status code.
	HTTPStatusCodes []int
}

var routeDescriptors = []RouteDescriptor{
	{
		Name:        RouteNameBase,
		Path:        "/v2/",
		Entity:      "Base",
		Description: `Base V2 API route. Typically, this can be used for lightweight version checks and to validate registry authorization.`,
		Methods: []MethodDescriptor{
			{
				Method:      "GET",
				Description: "Check that the endpoint implements Docker Registry API V2.",
				Requests: []RequestDescriptor{
					{
						Headers: []ParameterDescriptor{
							authHeader,
						},
						Successes: []ResponseDescriptor{
							{
								Description: "The API implements V2 protocol and is accessible.",
								StatusCode:  http.StatusOK,
							},
						},
						Failures: []ResponseDescriptor{
							{
								Description: "The client is not authorized to access the registry.",
								StatusCode:  http.StatusUnauthorized,
								Headers: []ParameterDescriptor{
									authChallengeHeader,
								},
							},
							{
								Description: "The registry does not implement the V2 API.",
								StatusCode:  http.StatusNotFound,
							},
						},
					},
				},
			},
		},
	},
	{
		Name:        RouteNameTags,
		Path:        "/v2/{name:" + RepositoryNameRegexp.String() + "}/tags/list",
		Entity:      "Tags",
		Description: "Retrieve information about tags.",
		Methods: []MethodDescriptor{
			{
				Method:      "GET",
				Description: "Fetch the tags under the repository identified by `name`.",
				Requests: []RequestDescriptor{
					{
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode:  http.StatusOK,
								Description: "A list of tags for the named repository.",
								Body: BodyDescriptor{
									ContentType: "application/json",
									Format: `{
    "name": <name>,
    "tags": [
        <tag>,
        ...
    ]
}`,
								},
							},
						},
						Failures: []ResponseDescriptor{
							{
								StatusCode:  http.StatusNotFound,
								Description: "The repository is not known to the registry.",
							},
							{
								StatusCode:  http.StatusUnauthorized,
								Description: "The client doesn't have access to repository.",
							},
						},
					},
				},
			},
		},
	},
	{
		Name:        RouteNameManifest,
		Path:        "/v2/{name:" + RepositoryNameRegexp.String() + "}/manifests/{tag:" + TagNameRegexp.String() + "}",
		Entity:      "Manifest",
		Description: "Create, update and retrieve manifests.",
		Methods: []MethodDescriptor{
			{
				Method:      "GET",
				Description: "Fetch the manifest identified by `name` and `tag`.",
				Requests: []RequestDescriptor{
					{
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							tagParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								Description: "The manifest idenfied by `name` and `tag`.",
								StatusCode:  http.StatusOK,
								Body: BodyDescriptor{
									ContentType: "application/json",
									Format:      manifestBody,
								},
							},
						},
						Failures: []ResponseDescriptor{
							{
								Description: "The name or tag was invalid.",
								StatusCode:  http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameInvalid,
									ErrorCodeTagInvalid,
								},
								Body: BodyDescriptor{
									ContentType: "application/json",
									Format:      errorsBody,
								},
							},
							{
								Description: "The named manifest is not known to the registry.",
								StatusCode:  http.StatusNotFound,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameUnknown,
									ErrorCodeManifestUnknown,
								},
								Body: BodyDescriptor{
									ContentType: "application/json",
									Format:      errorsBody,
								},
							},
						},
					},
				},
			},
			{
				Method:      "PUT",
				Description: "Put the manifest identified by `name` and `tag`.",
				Requests: []RequestDescriptor{
					{
						Headers: []ParameterDescriptor{
							authHeader,
						},
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							tagParameterDescriptor,
						},
						Body: BodyDescriptor{
							ContentType: "application/json",
							Format:      manifestBody,
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusAccepted,
							},
						},
						Failures: []ResponseDescriptor{
							{
								StatusCode: http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameInvalid,
									ErrorCodeTagInvalid,
									ErrorCodeManifestInvalid,
									ErrorCodeManifestUnverified,
									ErrorCodeBlobUnknown,
								},
							},
							{
								Description: "One or more layers may be missing during a manifest upload. If so, the missing layers will be enumerated in the error response.",
								StatusCode:  http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeBlobUnknown,
								},
								Body: BodyDescriptor{
									ContentType: "application/json",
									Format: `{
    "errors:" [{
            "code": "BLOB_UNKNOWN",
            "message": "blob unknown to registry",
            "detail": {
                "digest": <tarsum>
            }
        },
        ...
    ]
}`,
								},
							},
							{
								StatusCode: http.StatusUnauthorized,
								Headers: []ParameterDescriptor{
									authChallengeHeader,
								},
							},
						},
					},
				},
			},
			{
				Method:      "DELETE",
				Description: "Delete the manifest identified by `name` and `tag`.",
				Requests: []RequestDescriptor{
					{
						Headers: []ParameterDescriptor{
							authHeader,
						},
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							tagParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusAccepted,
							},
						},
						Failures: []ResponseDescriptor{
							{
								StatusCode: http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameInvalid,
									ErrorCodeTagInvalid,
								},
							},
							{
								StatusCode: http.StatusUnauthorized,
								Headers: []ParameterDescriptor{
									authChallengeHeader,
								},
							},
							{
								StatusCode: http.StatusNotFound,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameUnknown,
									ErrorCodeManifestUnknown,
								},
							},
						},
					},
				},
			},
		},
	},

	{
		Name:        RouteNameBlob,
		Path:        "/v2/{name:" + RepositoryNameRegexp.String() + "}/blobs/{digest:" + digest.DigestRegexp.String() + "}",
		Entity:      "Blob",
		Description: "Fetch the blob identified by `name` and `digest`. Used to fetch layers by tarsum digest.",
		Methods: []MethodDescriptor{

			{
				Method:      "GET",
				Description: "Retrieve the blob from the registry identified by `digest`.",
				Requests: []RequestDescriptor{
					{
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							digestPathParameter,
						},
						Successes: []ResponseDescriptor{
							{
								Description: "The blob identified by `digest` is available. The blob content will be present in the body of the request.",
								StatusCode:  http.StatusOK,
								Body: BodyDescriptor{
									ContentType: "application/octet-stream",
									Format:      "<blob binary data>",
								},
							},
							{
								Description: "The blob identified by `digest` is available at the provided location.",
								StatusCode:  http.StatusTemporaryRedirect,
								Headers: []ParameterDescriptor{
									{
										Name:        "Location",
										Type:        "url",
										Description: "The location where the layer should be accessible.",
										Format:      "<blob location>",
									},
								},
							},
						},
						Failures: []ResponseDescriptor{
							{
								StatusCode: http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameInvalid,
									ErrorCodeDigestInvalid,
								},
							},
							{
								StatusCode: http.StatusUnauthorized,
							},
							{
								StatusCode: http.StatusNotFound,
								ErrorCodes: []ErrorCode{
									ErrorCodeNameUnknown,
									ErrorCodeBlobUnknown,
								},
							},
						},
					},
				},
			},
			{
				Method:      "HEAD",
				Description: "Check if the blob is known to the registry.",
				Requests: []RequestDescriptor{
					{
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							digestPathParameter,
						},
					},
				},
			},
			// TODO(stevvooe): We may want to add a PUT request here to
			// kickoff an upload of a blob, integrated with the blob upload
			// API.
		},
	},

	{
		Name:        RouteNameBlobUpload,
		Path:        "/v2/{name:" + RepositoryNameRegexp.String() + "}/blobs/uploads/",
		Entity:      "Intiate Blob Upload",
		Description: "Initiate a blob upload. This endpoint can be used to create resumable uploads or monolithic uploads.",
		Methods: []MethodDescriptor{
			{
				Method:      "POST",
				Description: "Initiate a resumable blob upload. If successful, an upload location will be provided to complete the upload. Optionally, if the `digest` parameter is present, the request body will be used to complete the upload in a single request.",
				Requests: []RequestDescriptor{
					{
						Name:        "Initiate Monolithic Blob Upload",
						Description: "Upload a blob identified by the `digest` parameter in single request. This upload will not be resumable unless a recoverable error is returned.",
						Headers: []ParameterDescriptor{
							authHeader,
							{
								Name:   "Content-Length",
								Type:   "integer",
								Format: "<length of blob>",
							},
						},
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
						},
						QueryParameters: []ParameterDescriptor{
							{
								Name:        "digest",
								Type:        "query",
								Format:      "<tarsum>",
								Regexp:      digest.DigestRegexp,
								Description: `Digest of uploaded blob. If present, the upload will be completed, in a single request, with contents of the request body as the resulting blob.`,
							},
						},
						Body: BodyDescriptor{
							ContentType: "application/octect-stream",
							Format:      "<binary data>",
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusCreated,
								Headers: []ParameterDescriptor{
									{
										Name:   "Location",
										Type:   "url",
										Format: "<blob location>",
									},
									contentLengthZeroHeader,
								},
							},
						},
						Failures: []ResponseDescriptor{
							{
								Name:       "Invalid Name or Digest",
								StatusCode: http.StatusBadRequest,
								ErrorCodes: []ErrorCode{
									ErrorCodeDigestInvalid,
									ErrorCodeNameInvalid,
								},
							},
							{
								Name:       "Unauthorized",
								StatusCode: http.StatusUnauthorized,
								Headers: []ParameterDescriptor{
									authChallengeHeader,
								},
								ErrorCodes: []ErrorCode{
									ErrorCodeDigestInvalid,
									ErrorCodeNameInvalid,
								},
							},
						},
					},
					{
						Name:        "Initiate Resumable Blob Upload",
						Description: "Initiate a resumable blob upload with an empty request body.",
						Headers: []ParameterDescriptor{
							authHeader,
							contentLengthZeroHeader,
						},
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								Description: "The upload has been created. The `Location` header must be used to complete the upload. The response should identical to a `GET` request on the contents of the returned `Location` header.",
								StatusCode:  http.StatusAccepted,
								Headers: []ParameterDescriptor{
									contentLengthZeroHeader,
									{
										Name:        "Location",
										Type:        "url",
										Format:      "/v2/<name>/blobs/uploads/<uuid>",
										Description: "The location of the created upload. Clients should use the contents verbatim to complete the upload, adding parameters where required.",
									},
									{
										Name:        "Range",
										Format:      "0-0",
										Description: "Range header indicating the progress of the upload. When starting an upload, it will return an empty range, since no content has been received.",
									},
								},
							},
						},
					},
				},
			},
		},
	},

	{
		Name:        RouteNameBlobUploadChunk,
		Path:        "/v2/{name:" + RepositoryNameRegexp.String() + "}/blobs/uploads/{uuid}",
		Entity:      "Blob Upload",
		Description: "Interact with blob uploads. Clients should never assemble URLs for this endpoint and should only take it through the `Location` header on related API requests.",
		Methods: []MethodDescriptor{
			{
				Method:      "GET",
				Description: "Retrieve status of upload identified by `uuid`. The primary purpose of this endpoint is to resolve the current status of a resumable upload.",
				Requests: []RequestDescriptor{
					{
						Description: "Retrieve the progress of the current upload, as reported by the `Range` header.",
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							uuidParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusNoContent,
								Headers: []ParameterDescriptor{
									{
										Name:        "Range",
										Type:        "header",
										Format:      "0-<offset>",
										Description: "Range indicating the current progress of the upload.",
									},
								},
							},
						},
					},
				},
			},
			{
				Method:      "HEAD",
				Description: "Retrieve status of upload identified by `uuid`. This is identical to the GET request.",
				Requests: []RequestDescriptor{
					{
						Description: "Retrieve the progress of the current upload, as reported by the `Range` header.",
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							uuidParameterDescriptor,
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusNoContent,
								Headers: []ParameterDescriptor{
									{
										Name:        "Range",
										Type:        "header",
										Format:      "0-<offset>",
										Description: "Range indicating the current progress of the upload.",
									},
								},
							},
						},
					},
				},
			},
			{
				Method:      "PATCH",
				Description: "Upload a chunk of data for the specified upload.",
				Requests: []RequestDescriptor{
					{
						Description: "Upload a chunk of data to specified upload without completing the upload.",
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							uuidParameterDescriptor,
						},
						Headers: []ParameterDescriptor{
							{
								Name:        "Content-Range",
								Type:        "header",
								Format:      "<start of range>-<end of range, inclusive>",
								Required:    true,
								Description: "Range of bytes identifying the desired block of content represented by the body. Start must the end offset retrieved via status check plus one. Note that this is a non-standard use of the `Content-Range` header.",
							},
							{
								Name:        "Content-Length",
								Type:        "integer",
								Format:      "<length of chunk>",
								Description: "Length of the chunk being uploaded, corresponding the length of the request body.",
							},
						},
						Body: BodyDescriptor{
							ContentType: "application/octet-stream",
							Format:      "<binary chunk>",
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusNoContent,
								Headers: []ParameterDescriptor{
									{
										Name:        "Range",
										Type:        "header",
										Format:      "0-<offset>",
										Description: "Range indicating the current progress of the upload.",
									},
									contentLengthZeroHeader,
								},
							},
						},
					},
				},
			},
			{
				Method:      "PUT",
				Description: "Complete the upload specified by `uuid`, optionally appending the body as the final chunk.",
				Requests: []RequestDescriptor{
					{
						// TODO(stevvooe): Break this down into three separate requests:
						// 	1. Complete an upload where all data has already been sent.
						// 	2. Complete an upload where the entire body is in the PUT.
						// 	3. Complete an upload where the final, partial chunk is the body.

						Description: "Upload the _final_ chunk of data.",
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							uuidParameterDescriptor,
						},
						QueryParameters: []ParameterDescriptor{
							{
								Name:        "digest",
								Type:        "string",
								Format:      "<tarsum>",
								Regexp:      digest.DigestRegexp,
								Required:    true,
								Description: `Digest of uploaded blob.`,
							},
						},
						Successes: []ResponseDescriptor{
							{
								StatusCode: http.StatusNoContent,
								Headers: []ParameterDescriptor{
									{
										Name:        "Content-Range",
										Type:        "header",
										Format:      "<start of range>-<end of range, inclusive>",
										Description: "Range of bytes identifying the desired block of content represented by the body. Start must match the end of offset retrieved via status check. Note that this is a non-standard use of the `Content-Range` header.",
									},
									{
										Name:        "Content-Length",
										Type:        "integer",
										Format:      "<length of chunk>",
										Description: "Length of the chunk being uploaded, corresponding the length of the request body.",
									},
								},
								Body: BodyDescriptor{
									ContentType: "application/octet-stream",
									Format:      "<binary chunk>",
								},
							},
						},
					},
				},
			},
			{
				Method:      "DELETE",
				Description: "Cancel outstanding upload processes, releasing associated resources. If this is not called, the unfinished uploads will eventually timeout.",
				Requests: []RequestDescriptor{
					{
						Description: "Cancel the upload specified by `uuid`.",
						PathParameters: []ParameterDescriptor{
							nameParameterDescriptor,
							uuidParameterDescriptor,
						},
					},
				},
			},
		},
	},
}

// ErrorDescriptors provides a list of HTTP API Error codes that may be
// encountered when interacting with the registry API.
var errorDescriptors = []ErrorDescriptor{
	{
		Code:    ErrorCodeUnknown,
		Value:   "UNKNOWN",
		Message: "unknown error",
		Description: `Generic error returned when the error does not have an
		API classification.`,
	},
	{
		Code:    ErrorCodeUnauthorized,
		Value:   "UNAUTHORIZED",
		Message: "access to the requested resource is not authorized",
		Description: `The access controller denied access for the operation on
		a resource. Often this will be accompanied by a 401 Unauthorized
		response status.`,
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
		HTTPStatusCodes: []int{http.StatusBadRequest, http.StatusNotFound},
	},
	{
		Code:    ErrorCodeSizeInvalid,
		Value:   "SIZE_INVALID",
		Message: "provided length did not match content length",
		Description: `When a layer is uploaded, the provided size will be
		checked against the uploaded content. If they do not match, this error
		will be returned.`,
		HTTPStatusCodes: []int{http.StatusBadRequest},
	},
	{
		Code:    ErrorCodeNameInvalid,
		Value:   "NAME_INVALID",
		Message: "manifest name did not match URI",
		Description: `During a manifest upload, if the name in the manifest
		does not match the uri name, this error will be returned.`,
		HTTPStatusCodes: []int{http.StatusBadRequest, http.StatusNotFound},
	},
	{
		Code:    ErrorCodeTagInvalid,
		Value:   "TAG_INVALID",
		Message: "manifest tag did not match URI",
		Description: `During a manifest upload, if the tag in the manifest
		does not match the uri tag, this error will be returned.`,
		HTTPStatusCodes: []int{http.StatusBadRequest, http.StatusNotFound},
	},
	{
		Code:    ErrorCodeNameUnknown,
		Value:   "NAME_UNKNOWN",
		Message: "repository name not known to registry",
		Description: `This is returned if the name used during an operation is
		unknown to the registry.`,
		HTTPStatusCodes: []int{http.StatusNotFound},
	},
	{
		Code:    ErrorCodeManifestUnknown,
		Value:   "MANIFEST_UNKNOWN",
		Message: "manifest unknown",
		Description: `This error is returned when the manifest, identified by
		name and tag is unknown to the repository.`,
		HTTPStatusCodes: []int{http.StatusNotFound},
	},
	{
		Code:    ErrorCodeManifestInvalid,
		Value:   "MANIFEST_INVALID",
		Message: "manifest invalid",
		Description: `During upload, manifests undergo several checks ensuring
		validity. If those checks fail, this error may be returned, unless a
		more specific error is included. The detail will contain information
		the failed validation.`,
		HTTPStatusCodes: []int{http.StatusBadRequest},
	},
	{
		Code:    ErrorCodeManifestUnverified,
		Value:   "MANIFEST_UNVERIFIED",
		Message: "manifest failed signature verification",
		Description: `During manifest upload, if the manifest fails signature
		verification, this error will be returned.`,
		HTTPStatusCodes: []int{http.StatusBadRequest},
	},
	{
		Code:    ErrorCodeBlobUnknown,
		Value:   "BLOB_UNKNOWN",
		Message: "blob unknown to registry",
		Description: `This error may be returned when a blob is unknown to the
		registry in a specified repository. This can be returned with a
		standard get or if a manifest references an unknown layer during
		upload.`,
		HTTPStatusCodes: []int{http.StatusBadRequest, http.StatusNotFound},
	},

	{
		Code:    ErrorCodeBlobUploadUnknown,
		Value:   "BLOB_UPLOAD_UNKNOWN",
		Message: "blob upload unknown to registry",
		Description: `If a blob upload has been cancelled or was never
		started, this error code may be returned.`,
		HTTPStatusCodes: []int{http.StatusNotFound},
	},
}

var errorCodeToDescriptors map[ErrorCode]ErrorDescriptor
var idToDescriptors map[string]ErrorDescriptor

func init() {
	errorCodeToDescriptors = make(map[ErrorCode]ErrorDescriptor, len(errorDescriptors))
	idToDescriptors = make(map[string]ErrorDescriptor, len(errorDescriptors))

	for _, descriptor := range errorDescriptors {
		errorCodeToDescriptors[descriptor.Code] = descriptor
		idToDescriptors[descriptor.Value] = descriptor
	}
}
