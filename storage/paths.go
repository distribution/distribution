package storage

import (
	"fmt"
	"path"
	"strings"

	"github.com/docker/docker-registry/common"
	"github.com/docker/docker-registry/digest"
)

const storagePathVersion = "v2"

// TODO(sday): This needs to be changed: all layers for an image will be
// linked under the repository. Lookup from tarsum to name is not necessary,
// so we can remove the layer index. For this to properly work, image push
// must link the images layers under the repo.

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
// 						-> tarsum/
// 							-> <tarsum version>/
// 								-> <tarsum hash alg>/
// 									<layer links to blob store>
//			-> layerindex/
//				-> tarsum/
// 					-> 	<tarsum version>/
// 						-> <tarsum hash alg>/
// 							<repo name links>
//			-> blob/sha256
//				<split directory sha256 content addressable storage>
//
// There are few important components to this path layout. First, we have the
// repository store identified by name. This contains the image manifests and
// a layer store with links to CAS blob ids. Outside of the named repo area,
// we have the layerindex, which provides lookup from tarsum id to repo
// storage. The blob store contains the actual layer data and any other data
// that can be referenced by a CAS id.
//
// We cover the path formats implemented by this path mapper below.
//
// 	manifestPathSpec: <root>/v2/repositories/<name>/manifests/<tag>
// 	layerLinkPathSpec: <root>/v2/repositories/<name>/layers/tarsum/<tarsum version>/<tarsum hash alg>/<tarsum hash>
//	layerIndexLinkPathSpec: <root>/v2/layerindex/tarsum/<tarsum version>/<tarsum hash alg>/<tarsum hash>
// 	blobPathSpec: <root>/v2/blob/sha256/<first two hex bytes of digest>/<hex digest>
//
// For more information on the semantic meaning of each path and their
// contents, please see the path spec documentation.
type pathMapper struct {
	root    string
	version string // should be a constant?
}

// TODO(stevvooe): This storage layout currently allows lookup to layer stores
// by repo name via the tarsum. The layer index lookup could come with an
// access control check against the link contents before proceeding. The main
// problem with this comes with a collision in the tarsum algorithm: if party
// A uploads a layer before party B, with an identical tarsum, party B may
// never be able to get access to the tarsum stored under party A. We'll need
// a way for party B to associate with a "unique" version of their image. This
// may be as simple as forcing the client to re-upload images to which they
// don't have access.

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
	case manifestPathSpec:
		// TODO(sday): May need to store manifest by architecture.
		return path.Join(append(repoPrefix, v.name, "manifests", v.tag)...), nil
	case layerLinkPathSpec:
		if !strings.HasPrefix(v.digest.Algorithm(), "tarsum") {
			// Only tarsum is supported, for now
			return "", fmt.Errorf("unsupport content digest: %v", v.digest)
		}

		tsi, err := common.ParseTarSum(v.digest.String())

		if err != nil {
			// TODO(sday): This will return an InvalidTarSumError from
			// ParseTarSum but we may want to wrap this. This error should
			// never be encountered in production, since the tarsum should be
			// validated by this point.
			return "", err
		}

		return path.Join(append(append(repoPrefix, v.name, "layers"),
			tarSumInfoPathComponents(tsi)...)...), nil
	case layerIndexLinkPathSpec:
		if !strings.HasPrefix(v.digest.Algorithm(), "tarsum") {
			// Only tarsum is supported, for now
			return "", fmt.Errorf("unsupport content digest: %v", v.digest)
		}

		tsi, err := common.ParseTarSum(v.digest.String())

		if err != nil {
			// TODO(sday): This will return an InvalidTarSumError from
			// ParseTarSum but we may want to wrap this. This error should
			// never be encountered in production, since the tarsum should be
			// validated by this point.
			return "", err
		}

		return path.Join(append(append(rootPrefix, "layerindex"),
			tarSumInfoPathComponents(tsi)...)...), nil
	case blobPathSpec:
		p := path.Join([]string{pm.root, pm.version, "blob", v.alg, v.digest[:2], v.digest}...)
		return p, nil
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

// layerIndexLinkPath provides a path to a registry global layer store,
// indexed by tarsum. The target file will contain the repo name of the
// "owner" of the layer. An example name link file follows:
//
// 	library/ubuntu
// 	foo/bar
//
// The above file has the tarsum stored under the foo/bar repository and the
// library/ubuntu repository. The storage layer should access the tarsum from
// the first repository to which the client has access.
type layerIndexLinkPathSpec struct {
	digest digest.Digest
}

func (layerIndexLinkPathSpec) pathSpec() {}

// blobPath contains the path for the registry global blob store. For now,
// this contains layer data, exclusively.
type blobPathSpec struct {
	// TODO(stevvooe): Port this to make better use of Digest type.
	alg    string
	digest string
}

func (blobPathSpec) pathSpec() {}

// tarSumInfoPath generates storage path components for the provided
// TarSumInfo.
func tarSumInfoPathComponents(tsi common.TarSumInfo) []string {
	version := tsi.Version

	if version == "" {
		version = "v0"
	}

	return []string{"tarsum", version, tsi.Algorithm, tsi.Digest}
}
