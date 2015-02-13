package distribution

import (
	"fmt"
	"strings"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
)

var (
	// ErrLayerExists returned when layer already exists
	ErrLayerExists = fmt.Errorf("layer exists")

	// ErrLayerTarSumVersionUnsupported when tarsum is unsupported version.
	ErrLayerTarSumVersionUnsupported = fmt.Errorf("unsupported tarsum version")

	// ErrLayerUploadUnknown returned when upload is not found.
	ErrLayerUploadUnknown = fmt.Errorf("layer upload unknown")

	// ErrLayerClosed returned when an operation is attempted on a closed
	// Layer or LayerUpload.
	ErrLayerClosed = fmt.Errorf("layer closed")
)

// ErrRepositoryUnknown is returned if the named repository is not known by
// the registry.
type ErrRepositoryUnknown struct {
	Name string
}

func (err ErrRepositoryUnknown) Error() string {
	return fmt.Sprintf("unknown respository name=%s", err.Name)
}

// ErrRepositoryNameInvalid should be used to denote an invalid repository
// name. Reason may set, indicating the cause of invalidity.
type ErrRepositoryNameInvalid struct {
	Name   string
	Reason error
}

func (err ErrRepositoryNameInvalid) Error() string {
	return fmt.Sprintf("repository name %q invalid: %v", err.Name, err.Reason)
}

// ErrManifestUnknown is returned if the manifest is not known by the
// registry.
type ErrManifestUnknown struct {
	Name string
	Tag  string
}

func (err ErrManifestUnknown) Error() string {
	return fmt.Sprintf("unknown manifest name=%s tag=%s", err.Name, err.Tag)
}

// ErrUnknownManifestRevision is returned when a manifest cannot be found by
// revision within a repository.
type ErrUnknownManifestRevision struct {
	Name     string
	Revision digest.Digest
}

func (err ErrUnknownManifestRevision) Error() string {
	return fmt.Sprintf("unknown manifest name=%s revision=%s", err.Name, err.Revision)
}

// ErrManifestUnverified is returned when the registry is unable to verify
// the manifest.
type ErrManifestUnverified struct{}

func (ErrManifestUnverified) Error() string {
	return fmt.Sprintf("unverified manifest")
}

// ErrManifestVerification provides a type to collect errors encountered
// during manifest verification. Currently, it accepts errors of all types,
// but it may be narrowed to those involving manifest verification.
type ErrManifestVerification []error

func (errs ErrManifestVerification) Error() string {
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}

	return fmt.Sprintf("errors verifying manifest: %v", strings.Join(parts, ","))
}

// ErrUnknownLayer returned when layer cannot be found.
type ErrUnknownLayer struct {
	FSLayer manifest.FSLayer
}

func (err ErrUnknownLayer) Error() string {
	return fmt.Sprintf("unknown layer %v", err.FSLayer.BlobSum)
}

// ErrLayerInvalidDigest returned when tarsum check fails.
type ErrLayerInvalidDigest struct {
	Digest digest.Digest
	Reason error
}

func (err ErrLayerInvalidDigest) Error() string {
	return fmt.Sprintf("invalid digest for referenced layer: %v, %v",
		err.Digest, err.Reason)
}
