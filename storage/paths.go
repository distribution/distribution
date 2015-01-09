package storage

import (
	"fmt"
	"path"
	"strings"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver"
)

const storagePathVersion = "v2"

// pathMapper maps paths based on "object names" and their ids. The "object
// names" mapped by pathMapper are internal to the storage system.
//
// The path layout in the storage backend will be roughly as follows:
//
//		<root>/v2
//			-> repositories/
// 				-><name>/
// 					-> manifests/
// 						<manifests by tag name>
// 					-> layers/
// 						<layer links to blob store>
// 					-> uploads/<uuid>
// 						data
// 						startedat
//			-> blob/<algorithm>
//				<split directory content addressable storage>
//
// There are few important components to this path layout. First, we have the
// repository store identified by name. This contains the image manifests and
// a layer store with links to CAS blob ids. Upload coordination data is also
// stored here. Outside of the named repo area, we have the the blob store. It
// contains the actual layer data and any other data that can be referenced by
// a CAS id.
//
// We cover the path formats implemented by this path mapper below.
//
// 	manifestPathSpec: <root>/v2/repositories/<name>/manifests/<tag>
// 	layerLinkPathSpec: <root>/v2/repositories/<name>/layers/tarsum/<tarsum version>/<tarsum hash alg>/<tarsum hash>
// 	blobPathSpec: <root>/v2/blob/<algorithm>/<first two hex bytes of digest>/<hex digest>
// 	uploadDataPathSpec: <root>/v2/repositories/<name>/uploads/<uuid>/data
// 	uploadStartedAtPathSpec: <root>/v2/repositories/<name>/uploads/<uuid>/startedat
//
// For more information on the semantic meaning of each path and their
// contents, please see the path spec documentation.
type pathMapper struct {
	root    string
	version string // should be a constant?
}

var defaultPathMapper = &pathMapper{
	root:    "/docker/registry/",
	version: storagePathVersion,
}

// path returns the path identified by spec.
func (pm *pathMapper) path(spec pathSpec) (string, error) {

	// Switch on the path object type and return the appropriate path. At
	// first glance, one may wonder why we don't use an interface to
	// accomplish this. By keep the formatting separate from the pathSpec, we
	// keep separate the path generation componentized. These specs could be
	// passed to a completely different mapper implementation and generate a
	// different set of paths.
	//
	// For example, imagine migrating from one backend to the other: one could
	// build a filesystem walker that converts a string path in one version,
	// to an intermediate path object, than can be consumed and mapped by the
	// other version.

	rootPrefix := []string{pm.root, pm.version}
	repoPrefix := append(rootPrefix, "repositories")

	switch v := spec.(type) {
	case manifestTagsPath:
		return path.Join(append(repoPrefix, v.name, "manifests")...), nil
	case manifestPathSpec:
		// TODO(sday): May need to store manifest by architecture.
		return path.Join(append(repoPrefix, v.name, "manifests", v.tag)...), nil
	case layerLinkPathSpec:
		components, err := digestPathComoponents(v.digest)
		if err != nil {
			return "", err
		}

		// For now, only map tarsum paths.
		if components[0] != "tarsum" {
			// Only tarsum is supported, for now
			return "", fmt.Errorf("unsupported content digest: %v", v.digest)
		}

		layerLinkPathComponents := append(repoPrefix, v.name, "layers")

		return path.Join(append(layerLinkPathComponents, components...)...), nil
	case blobPathSpec:
		components, err := digestPathComoponents(v.digest)
		if err != nil {
			return "", err
		}

		// For now, only map tarsum paths.
		if components[0] != "tarsum" {
			// Only tarsum is supported, for now
			return "", fmt.Errorf("unsupported content digest: %v", v.digest)
		}

		blobPathPrefix := append(rootPrefix, "blob")
		return path.Join(append(blobPathPrefix, components...)...), nil
	case uploadDataPathSpec:
		return path.Join(append(repoPrefix, v.name, "uploads", v.uuid, "data")...), nil
	case uploadStartedAtPathSpec:
		return path.Join(append(repoPrefix, v.name, "uploads", v.uuid, "startedat")...), nil
	default:
		// TODO(sday): This is an internal error. Ensure it doesn't escape (panic?).
		return "", fmt.Errorf("unknown path spec: %#v", v)
	}
}

// pathSpec is a type to mark structs as path specs. There is no
// implementation because we'd like to keep the specs and the mappers
// decoupled.
type pathSpec interface {
	pathSpec()
}

// manifestTagsPath describes the path elements required to point to the
// directory with all manifest tags under the repository.
type manifestTagsPath struct {
	name string
}

func (manifestTagsPath) pathSpec() {}

// manifestPathSpec describes the path elements used to build a manifest path.
// The contents should be a signed manifest json file.
type manifestPathSpec struct {
	name string
	tag  string
}

func (manifestPathSpec) pathSpec() {}

// layerLink specifies a path for a layer link, which is a file with a blob
// id. The layer link will contain a content addressable blob id reference
// into the blob store. The format of the contents is as follows:
//
// 	<algorithm>:<hex digest of layer data>
//
// The following example of the file contents is more illustrative:
//
// 	sha256:96443a84ce518ac22acb2e985eda402b58ac19ce6f91980bde63726a79d80b36
//
// This says indicates that there is a blob with the id/digest, calculated via
// sha256 that can be fetched from the blob store.
type layerLinkPathSpec struct {
	name   string
	digest digest.Digest
}

func (layerLinkPathSpec) pathSpec() {}

// blobAlgorithmReplacer does some very simple path sanitization for user
// input. Mostly, this is to provide some heirachry for tarsum digests. Paths
// should be "safe" before getting this far due to strict digest requirements
// but we can add further path conversion here, if needed.
var blobAlgorithmReplacer = strings.NewReplacer(
	"+", "/",
	".", "/",
	";", "/",
)

// blobPath contains the path for the registry global blob store. For now,
// this contains layer data, exclusively.
type blobPathSpec struct {
	digest digest.Digest
}

func (blobPathSpec) pathSpec() {}

// uploadDataPathSpec defines the path parameters of the data file for
// uploads.
type uploadDataPathSpec struct {
	name string
	uuid string
}

func (uploadDataPathSpec) pathSpec() {}

// uploadDataPathSpec defines the path parameters for the file that stores the
// start time of an uploads. If it is missing, the upload is considered
// unknown. Admittedly, the presence of this file is an ugly hack to make sure
// we have a way to cleanup old or stalled uploads that doesn't rely on driver
// FileInfo behavior. If we come up with a more clever way to do this, we
// should remove this file immediately and rely on the startetAt field from
// the client to enforce time out policies.
type uploadStartedAtPathSpec struct {
	name string
	uuid string
}

func (uploadStartedAtPathSpec) pathSpec() {}

// digestPathComoponents provides a consistent path breakdown for a given
// digest. For a generic digest, it will be as follows:
//
// 	<algorithm>/<first two bytes of digest>/<full digest>
//
// Most importantly, for tarsum, the layout looks like this:
//
// 	tarsum/<version>/<digest algorithm>/<first two bytes of digest>/<full digest>
//
// This is slightly specialized to store an extra version path for version 0
// tarsums.
func digestPathComoponents(dgst digest.Digest) ([]string, error) {
	if err := dgst.Validate(); err != nil {
		return nil, err
	}

	algorithm := blobAlgorithmReplacer.Replace(dgst.Algorithm())
	hex := dgst.Hex()
	prefix := []string{algorithm}
	suffix := []string{
		hex[:2], // Breaks heirarchy up.
		hex,
	}

	if tsi, err := digest.ParseTarSum(dgst.String()); err == nil {
		// We have a tarsum!
		version := tsi.Version
		if version == "" {
			version = "v0"
		}

		prefix = []string{
			"tarsum",
			version,
			tsi.Algorithm,
		}
	}

	return append(prefix, suffix...), nil
}

// resolveBlobPath looks up the blob location in the repositories from a
// layer/blob link file, returning blob path or an error on failure.
func resolveBlobPath(driver storagedriver.StorageDriver, pm *pathMapper, name string, dgst digest.Digest) (string, error) {
	pathSpec := layerLinkPathSpec{name: name, digest: dgst}
	layerLinkPath, err := pm.path(pathSpec)

	if err != nil {
		return "", err
	}

	layerLinkContent, err := driver.GetContent(layerLinkPath)
	if err != nil {
		return "", err
	}

	// NOTE(stevvooe): The content of the layer link should match the digest.
	// This layer of indirection is for name-based content protection.

	linked, err := digest.ParseDigest(string(layerLinkContent))
	if err != nil {
		return "", err
	}

	bp := blobPathSpec{digest: linked}

	return pm.path(bp)
}
