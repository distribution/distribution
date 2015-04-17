package storage

import (
	"fmt"
	"path"
	"strings"

	"github.com/docker/distribution/digest"
)

const storagePathVersion = "v2"

// pathMapper maps paths based on "object names" and their ids. The "object
// names" mapped by pathMapper are internal to the storage system.
//
// The path layout in the storage backend is roughly as follows:
//
//		<root>/v2
//			-> repositories/
// 				-><name>/
// 					-> _manifests/
// 						revisions
//							-> <manifest digest path>
//								-> link
//								-> signatures
// 									<algorithm>/<digest>/link
// 						tags/<tag>
//							-> current/link
// 							-> index
//								-> <algorithm>/<hex digest>/link
// 					-> _layers/
// 						<layer links to blob store>
// 					-> _uploads/<uuid>
// 						data
// 						startedat
// 						hashstates/<algorithm>/<offset>
//			-> blob/<algorithm>
//				<split directory content addressable storage>
//
// The storage backend layout is broken up into a content- addressable blob
// store and repositories. The content-addressable blob store holds most data
// throughout the backend, keyed by algorithm and digests of the underlying
// content. Access to the blob store is controled through links from the
// repository to blobstore.
//
// A repository is made up of layers, manifests and tags. The layers component
// is just a directory of layers which are "linked" into a repository. A layer
// can only be accessed through a qualified repository name if it is linked in
// the repository. Uploads of layers are managed in the uploads directory,
// which is key by upload uuid. When all data for an upload is received, the
// data is moved into the blob store and the upload directory is deleted.
// Abandoned uploads can be garbage collected by reading the startedat file
// and removing uploads that have been active for longer than a certain time.
//
// The third component of the repository directory is the manifests store,
// which is made up of a revision store and tag store. Manifests are stored in
// the blob store and linked into the revision store. Signatures are separated
// from the manifest payload data and linked into the blob store, as well.
// While the registry can save all revisions of a manifest, no relationship is
// implied as to the ordering of changes to a manifest. The tag store provides
// support for name, tag lookups of manifests, using "current/link" under a
// named tag directory. An index is maintained to support deletions of all
// revisions of a given manifest tag.
//
// We cover the path formats implemented by this path mapper below.
//
//	Manifests:
//
// 	manifestRevisionPathSpec:      <root>/v2/repositories/<name>/_manifests/revisions/<algorithm>/<hex digest>/
// 	manifestRevisionLinkPathSpec:  <root>/v2/repositories/<name>/_manifests/revisions/<algorithm>/<hex digest>/link
// 	manifestSignaturesPathSpec:    <root>/v2/repositories/<name>/_manifests/revisions/<algorithm>/<hex digest>/signatures/
// 	manifestSignatureLinkPathSpec: <root>/v2/repositories/<name>/_manifests/revisions/<algorithm>/<hex digest>/signatures/<algorithm>/<hex digest>/link
//
//	Tags:
//
// 	manifestTagsPathSpec:                  <root>/v2/repositories/<name>/_manifests/tags/
// 	manifestTagPathSpec:                   <root>/v2/repositories/<name>/_manifests/tags/<tag>/
// 	manifestTagCurrentPathSpec:            <root>/v2/repositories/<name>/_manifests/tags/<tag>/current/link
// 	manifestTagIndexPathSpec:              <root>/v2/repositories/<name>/_manifests/tags/<tag>/index/
// 	manifestTagIndexEntryPathSpec:         <root>/v2/repositories/<name>/_manifests/tags/<tag>/index/<algorithm>/<hex digest>/
// 	manifestTagIndexEntryLinkPathSpec:     <root>/v2/repositories/<name>/_manifests/tags/<tag>/index/<algorithm>/<hex digest>/link
//
// 	Layers:
//
// 	layerLinkPathSpec:             <root>/v2/repositories/<name>/_layers/tarsum/<tarsum version>/<tarsum hash alg>/<tarsum hash>/link
//
//	Uploads:
//
// 	uploadDataPathSpec:             <root>/v2/repositories/<name>/_uploads/<uuid>/data
// 	uploadStartedAtPathSpec:        <root>/v2/repositories/<name>/_uploads/<uuid>/startedat
// 	uploadHashStatePathSpec:        <root>/v2/repositories/<name>/_uploads/<uuid>/hashstates/<algorithm>/<offset>
//
//	Blob Store:
//
// 	blobPathSpec:                   <root>/v2/blobs/<algorithm>/<first two hex bytes of digest>/<hex digest>
// 	blobDataPathSpec:               <root>/v2/blobs/<algorithm>/<first two hex bytes of digest>/<hex digest>/data
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

	case manifestRevisionPathSpec:
		components, err := digestPathComponents(v.revision, false)
		if err != nil {
			return "", err
		}

		return path.Join(append(append(repoPrefix, v.name, "_manifests", "revisions"), components...)...), nil
	case manifestRevisionLinkPathSpec:
		root, err := pm.path(manifestRevisionPathSpec{
			name:     v.name,
			revision: v.revision,
		})

		if err != nil {
			return "", err
		}

		return path.Join(root, "link"), nil
	case manifestSignaturesPathSpec:
		root, err := pm.path(manifestRevisionPathSpec{
			name:     v.name,
			revision: v.revision,
		})

		if err != nil {
			return "", err
		}

		return path.Join(root, "signatures"), nil
	case manifestSignatureLinkPathSpec:
		root, err := pm.path(manifestSignaturesPathSpec{
			name:     v.name,
			revision: v.revision,
		})
		if err != nil {
			return "", err
		}

		signatureComponents, err := digestPathComponents(v.signature, false)
		if err != nil {
			return "", err
		}

		return path.Join(root, path.Join(append(signatureComponents, "link")...)), nil
	case manifestTagsPathSpec:
		return path.Join(append(repoPrefix, v.name, "_manifests", "tags")...), nil
	case manifestTagPathSpec:
		root, err := pm.path(manifestTagsPathSpec{
			name: v.name,
		})
		if err != nil {
			return "", err
		}

		return path.Join(root, v.tag), nil
	case manifestTagCurrentPathSpec:
		root, err := pm.path(manifestTagPathSpec{
			name: v.name,
			tag:  v.tag,
		})
		if err != nil {
			return "", err
		}

		return path.Join(root, "current", "link"), nil
	case manifestTagIndexPathSpec:
		root, err := pm.path(manifestTagPathSpec{
			name: v.name,
			tag:  v.tag,
		})
		if err != nil {
			return "", err
		}

		return path.Join(root, "index"), nil
	case manifestTagIndexEntryLinkPathSpec:
		root, err := pm.path(manifestTagIndexEntryPathSpec{
			name:     v.name,
			tag:      v.tag,
			revision: v.revision,
		})
		if err != nil {
			return "", err
		}

		return path.Join(root, "link"), nil
	case manifestTagIndexEntryPathSpec:
		root, err := pm.path(manifestTagIndexPathSpec{
			name: v.name,
			tag:  v.tag,
		})
		if err != nil {
			return "", err
		}

		components, err := digestPathComponents(v.revision, false)
		if err != nil {
			return "", err
		}

		return path.Join(root, path.Join(components...)), nil
	case layerLinkPathSpec:
		components, err := digestPathComponents(v.digest, false)
		if err != nil {
			return "", err
		}

		layerLinkPathComponents := append(repoPrefix, v.name, "_layers")

		return path.Join(path.Join(append(layerLinkPathComponents, components...)...), "link"), nil
	case blobDataPathSpec:
		components, err := digestPathComponents(v.digest, true)
		if err != nil {
			return "", err
		}

		components = append(components, "data")
		blobPathPrefix := append(rootPrefix, "blobs")
		return path.Join(append(blobPathPrefix, components...)...), nil

	case uploadDataPathSpec:
		return path.Join(append(repoPrefix, v.name, "_uploads", v.uuid, "data")...), nil
	case uploadStartedAtPathSpec:
		return path.Join(append(repoPrefix, v.name, "_uploads", v.uuid, "startedat")...), nil
	case uploadHashStatePathSpec:
		offset := fmt.Sprintf("%d", v.offset)
		if v.list {
			offset = "" // Limit to the prefix for listing offsets.
		}
		return path.Join(append(repoPrefix, v.name, "_uploads", v.uuid, "hashstates", v.alg, offset)...), nil
	case repositoriesRootPathSpec:
		return path.Join(repoPrefix...), nil
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

// manifestRevisionPathSpec describes the components of the directory path for
// a manifest revision.
type manifestRevisionPathSpec struct {
	name     string
	revision digest.Digest
}

func (manifestRevisionPathSpec) pathSpec() {}

// manifestRevisionLinkPathSpec describes the path components required to look
// up the data link for a revision of a manifest. If this file is not present,
// the manifest blob is not available in the given repo. The contents of this
// file should just be the digest.
type manifestRevisionLinkPathSpec struct {
	name     string
	revision digest.Digest
}

func (manifestRevisionLinkPathSpec) pathSpec() {}

// manifestSignaturesPathSpec decribes the path components for the directory
// containing all the signatures for the target blob. Entries are named with
// the underlying key id.
type manifestSignaturesPathSpec struct {
	name     string
	revision digest.Digest
}

func (manifestSignaturesPathSpec) pathSpec() {}

// manifestSignatureLinkPathSpec decribes the path components used to look up
// a signature file by the hash of its blob.
type manifestSignatureLinkPathSpec struct {
	name      string
	revision  digest.Digest
	signature digest.Digest
}

func (manifestSignatureLinkPathSpec) pathSpec() {}

// manifestTagsPathSpec describes the path elements required to point to the
// manifest tags directory.
type manifestTagsPathSpec struct {
	name string
}

func (manifestTagsPathSpec) pathSpec() {}

// manifestTagPathSpec describes the path elements required to point to the
// manifest tag links files under a repository. These contain a blob id that
// can be used to look up the data and signatures.
type manifestTagPathSpec struct {
	name string
	tag  string
}

func (manifestTagPathSpec) pathSpec() {}

// manifestTagCurrentPathSpec describes the link to the current revision for a
// given tag.
type manifestTagCurrentPathSpec struct {
	name string
	tag  string
}

func (manifestTagCurrentPathSpec) pathSpec() {}

// manifestTagCurrentPathSpec describes the link to the index of revisions
// with the given tag.
type manifestTagIndexPathSpec struct {
	name string
	tag  string
}

func (manifestTagIndexPathSpec) pathSpec() {}

// manifestTagIndexEntryPathSpec contains the entries of the index by revision.
type manifestTagIndexEntryPathSpec struct {
	name     string
	tag      string
	revision digest.Digest
}

func (manifestTagIndexEntryPathSpec) pathSpec() {}

// manifestTagIndexEntryLinkPathSpec describes the link to a revisions of a
// manifest with given tag within the index.
type manifestTagIndexEntryLinkPathSpec struct {
	name     string
	tag      string
	revision digest.Digest
}

func (manifestTagIndexEntryLinkPathSpec) pathSpec() {}

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
// input. Mostly, this is to provide some hierarchy for tarsum digests. Paths
// should be "safe" before getting this far due to strict digest requirements
// but we can add further path conversion here, if needed.
var blobAlgorithmReplacer = strings.NewReplacer(
	"+", "/",
	".", "/",
	";", "/",
)

// // blobPathSpec contains the path for the registry global blob store.
// type blobPathSpec struct {
// 	digest digest.Digest
// }

// func (blobPathSpec) pathSpec() {}

// blobDataPathSpec contains the path for the registry global blob store. For
// now, this contains layer data, exclusively.
type blobDataPathSpec struct {
	digest digest.Digest
}

func (blobDataPathSpec) pathSpec() {}

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

// uploadHashStatePathSpec defines the path parameters for the file that stores
// the hash function state of an upload at a specific byte offset. If `list` is
// set, then the path mapper will generate a list prefix for all hash state
// offsets for the upload identified by the name, uuid, and alg.
type uploadHashStatePathSpec struct {
	name   string
	uuid   string
	alg    string
	offset int64
	list   bool
}

func (uploadHashStatePathSpec) pathSpec() {}

// repositoriesRootPathSpec returns the root of repositories
type repositoriesRootPathSpec struct {
}

func (repositoriesRootPathSpec) pathSpec() {}

// digestPathComponents provides a consistent path breakdown for a given
// digest. For a generic digest, it will be as follows:
//
// 	<algorithm>/<hex digest>
//
// Most importantly, for tarsum, the layout looks like this:
//
// 	tarsum/<version>/<digest algorithm>/<full digest>
//
// If multilevel is true, the first two bytes of the digest will separate
// groups of digest folder. It will be as follows:
//
// 	<algorithm>/<first two bytes of digest>/<full digest>
//
func digestPathComponents(dgst digest.Digest, multilevel bool) ([]string, error) {
	if err := dgst.Validate(); err != nil {
		return nil, err
	}

	algorithm := blobAlgorithmReplacer.Replace(dgst.Algorithm())
	hex := dgst.Hex()
	prefix := []string{algorithm}

	var suffix []string

	if multilevel {
		suffix = append(suffix, hex[:2])
	}

	suffix = append(suffix, hex)

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
