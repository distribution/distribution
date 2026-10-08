package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ReferrerResult represents a single referrer linked to a subject.
type ReferrerResult struct {
	Digest       digest.Digest
	ArtifactType string
}

// ReferenceService is a service to manage internal links from subjects back to
// their referrers.
type ReferenceService interface {
	// Link creates a link from a subject back to a referrer
	Link(ctx context.Context, mediaType string, referrer, subject digest.Digest) error

	// Referrers returns the descriptors of all manifests that reference the
	// given subject digest. If artifactType is non-empty, results are filtered
	// to only that type.
	Referrers(ctx context.Context, subject digest.Digest, artifactType string) ([]v1.Descriptor, error)
}

type referenceHandler struct {
	*blobStore
	repository distribution.Repository
	pathFn     func(name, mediaType string, reference, artifact_subject_must_be_manifest digest.Digest) (string, error)
}

func (r *referenceHandler) Link(ctx context.Context, artifactType string, referrer, subject digest.Digest) error {
	path, err := r.pathFn(r.repository.Named().Name(), artifactType, referrer, subject)
	if err != nil {
		return err
	}

	return r.blobStore.link(ctx, path, referrer)
}

// Referrers enumerates the _referrers directory for the given subject and
// returns a descriptor for each linked manifest. The storage layout is:
//
//	<repo>/_manifests/revisions/<subject-alg>/<subject-hex>/_referrers/_<artifactType>/<ref-alg>/<ref-hex>/link
func (r *referenceHandler) Referrers(ctx context.Context, subject digest.Digest, artifactType string) ([]v1.Descriptor, error) {
	name := r.repository.Named().Name()

	// Build the base path: <repo>/_manifests/revisions/<subject>/_referrers/
	manifestPath, err := pathFor(manifestRevisionPathSpec{name: name, revision: subject})
	if err != nil {
		return nil, err
	}
	referrersRoot := path.Join(manifestPath, "_referrers")

	// List artifact type directories under _referrers/
	typeDirs, err := r.blobStore.driver.List(ctx, referrersRoot)
	if err != nil {
		// If the directory doesn't exist, there are no referrers.
		if isPathNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing referrers: %w", err)
	}

	var descriptors []v1.Descriptor

	for _, typeDir := range typeDirs {
		dirName := path.Base(typeDir)
		if !strings.HasPrefix(dirName, "_") {
			continue
		}

		// Decode the artifact type: strip leading "_" and URL-unescape
		decodedType, err := url.QueryUnescape(dirName[1:])
		if err != nil {
			continue
		}

		// Filter by artifactType if requested
		if artifactType != "" && decodedType != artifactType {
			continue
		}

		// Walk the algorithm directories under this type dir to find link files.
		err = enumerateReferrerLinks(ctx, r.blobStore, typeDir, decodedType, &descriptors)
		if err != nil {
			return nil, err
		}
	}

	return descriptors, nil
}

// enumerateReferrerLinks walks <typeDir>/<algorithm>/<hex>/link entries and
// builds descriptors for each referrer found.
func enumerateReferrerLinks(ctx context.Context, bs *blobStore, typeDir, artifactType string, descriptors *[]v1.Descriptor) error {
	// List algorithm directories (e.g., "sha256")
	algDirs, err := bs.driver.List(ctx, typeDir)
	if err != nil {
		if isPathNotFound(err) {
			return nil
		}
		return err
	}

	for _, algDir := range algDirs {
		alg := path.Base(algDir)

		// List hex digest directories
		hexDirs, err := bs.driver.List(ctx, algDir)
		if err != nil {
			if isPathNotFound(err) {
				continue
			}
			return err
		}

		for _, hexDir := range hexDirs {
			hex := path.Base(hexDir)
			linkPath := path.Join(hexDir, "link")

			referrerDigest, err := bs.readlink(ctx, linkPath)
			if err != nil {
				if isPathNotFound(err) {
					continue
				}
				return err
			}

			// Verify consistency
			expected := digest.NewDigestFromEncoded(digest.Algorithm(alg), hex)
			if referrerDigest != expected {
				continue
			}

			// Stat the blob to get size
			desc, err := bs.statter.Stat(ctx, referrerDigest)
			if err != nil {
				// Referrer blob may have been deleted; skip it
				continue
			}

			// Read the manifest to extract mediaType and annotations
			// per the OCI Distribution Spec referrers response requirements.
			enrichDescriptorFromManifest(ctx, bs, referrerDigest, &desc, artifactType)

			*descriptors = append(*descriptors, desc)
		}
	}

	return nil
}

// manifestEnvelope is a minimal struct for extracting mediaType, artifactType,
// annotations, and config.mediaType from a stored manifest without fully
// deserializing it.
type manifestEnvelope struct {
	MediaType    string            `json:"mediaType,omitempty"`
	ArtifactType string            `json:"artifactType,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Config       *v1.Descriptor    `json:"config,omitempty"`
}

// enrichDescriptorFromManifest reads the raw manifest blob and populates the
// descriptor's MediaType, ArtifactType, and Annotations per the OCI
// Distribution Spec referrers response requirements.
func enrichDescriptorFromManifest(ctx context.Context, bs *blobStore, dgst digest.Digest, desc *v1.Descriptor, storedArtifactType string) {
	blobPath, err := pathFor(blobDataPathSpec{digest: dgst})
	if err != nil {
		desc.ArtifactType = storedArtifactType
		return
	}

	content, err := bs.driver.GetContent(ctx, blobPath)
	if err != nil {
		desc.ArtifactType = storedArtifactType
		return
	}

	var env manifestEnvelope
	if err := json.Unmarshal(content, &env); err != nil {
		desc.ArtifactType = storedArtifactType
		return
	}

	if env.MediaType != "" {
		desc.MediaType = env.MediaType
	}
	if env.Annotations != nil {
		desc.Annotations = env.Annotations
	}

	// ArtifactType: use manifest's artifactType, fall back to config.mediaType,
	// then fall back to the stored directory name.
	switch {
	case env.ArtifactType != "":
		desc.ArtifactType = env.ArtifactType
	case env.Config != nil && env.Config.MediaType != "" && env.Config.MediaType != v1.MediaTypeEmptyJSON:
		desc.ArtifactType = env.Config.MediaType
	default:
		desc.ArtifactType = storedArtifactType
	}
}

// isPathNotFound returns true if the error indicates the path does not exist.
func isPathNotFound(err error) bool {
	switch err.(type) {
	case driver.PathNotFoundError:
		return true
	}
	return false
}

// NewReferenceEnumerator creates a ReferenceService that can enumerate
// referrers for a given repository using the storage driver directly.
func NewReferenceEnumerator(d driver.StorageDriver, repo distribution.Repository) ReferenceService {
	bs := &blobStore{
		driver:  d,
		statter: &blobStatter{driver: d},
	}
	return &referenceHandler{
		blobStore:  bs,
		repository: repo,
		pathFn:     subjectReferrerLinkPath,
	}
}

// subjectReferrerLinkPath provides the path to the subject's referrer link
func subjectReferrerLinkPath(name, mediaType string, referrer, subject digest.Digest) (string, error) {
	return pathFor(subjectReferrerLinkPathSpec{name: name, mediaType: mediaType, referrer: referrer, subject: subject})
}
