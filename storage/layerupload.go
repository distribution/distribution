package storage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"code.google.com/p/go-uuid/uuid"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver"
	"github.com/docker/docker/pkg/tarsum"
)

// LayerUploadState captures the state serializable state of the layer upload.
type LayerUploadState struct {
	// name is the primary repository under which the layer will be linked.
	Name string

	// UUID identifies the upload.
	UUID string

	// offset contains the current progress of the upload.
	Offset int64
}

// layerUploadController is used to control the various aspects of resumable
// layer upload. It implements the LayerUpload interface.
type layerUploadController struct {
	LayerUploadState

	layerStore  *layerStore
	uploadStore layerUploadStore
	fp          layerFile
	err         error // terminal error, if set, controller is closed
}

// layerFile documents the interface used while writing layer files, similar
// to *os.File. This is separate from layerReader, for now, because we want to
// store uploads on the local file system until we have write-through hashing
// support. They should be combined once this is worked out.
type layerFile interface {
	io.WriteSeeker
	io.Reader
	io.Closer

	// Sync commits the contents of the writer to storage.
	Sync() (err error)
}

// layerUploadStore provides storage for temporary files and upload state of
// layers. This is be used by the LayerService to manage the state of ongoing
// uploads. This interface will definitely change and will most likely end up
// being exported to the app layer. Move the layer.go when it's ready to go.
type layerUploadStore interface {
	New(name string) (LayerUploadState, error)
	Open(uuid string) (layerFile, error)
	GetState(uuid string) (LayerUploadState, error)
	// TODO: factor this method back in
	// SaveState(lus LayerUploadState) error
	DeleteState(uuid string) error
}

var _ LayerUpload = &layerUploadController{}

// Name of the repository under which the layer will be linked.
func (luc *layerUploadController) Name() string {
	return luc.LayerUploadState.Name
}

// UUID returns the identifier for this upload.
func (luc *layerUploadController) UUID() string {
	return luc.LayerUploadState.UUID
}

// Offset returns the position of the last byte written to this layer.
func (luc *layerUploadController) Offset() int64 {
	return luc.LayerUploadState.Offset
}

// Finish marks the upload as completed, returning a valid handle to the
// uploaded layer. The final size and checksum are validated against the
// contents of the uploaded layer. The checksum should be provided in the
// format <algorithm>:<hex digest>.
func (luc *layerUploadController) Finish(size int64, digest digest.Digest) (Layer, error) {

	// This section is going to be pretty ugly now. We will have to read the
	// file twice. First, to get the tarsum and checksum. When those are
	// available, and validated, we will upload it to the blob store and link
	// it into the repository. In the future, we need to use resumable hash
	// calculations for tarsum and checksum that can be calculated during the
	// upload. This will allow us to cut the data directly into a temporary
	// directory in the storage backend.

	fp, err := luc.file()

	if err != nil {
		// Cleanup?
		return nil, err
	}

	digest, err = luc.validateLayer(fp, size, digest)
	if err != nil {
		return nil, err
	}

	if nn, err := luc.writeLayer(fp, digest); err != nil {
		// Cleanup?
		return nil, err
	} else if size >= 0 && nn != size {
		// TODO(stevvooe): Short write. Will have to delete the location and
		// report an error. This error needs to be reported to the client.
		return nil, fmt.Errorf("short write writing layer")
	}

	// Yes! We have written some layer data. Let's make it visible. Link the
	// layer blob into the repository.
	if err := luc.linkLayer(digest); err != nil {
		return nil, err
	}

	// Ok, the upload has completed and finished. Delete the state.
	if err := luc.uploadStore.DeleteState(luc.UUID()); err != nil {
		// Can we ignore this error?
		return nil, err
	}

	return luc.layerStore.Fetch(luc.Name(), digest)
}

// Cancel the layer upload process.
func (luc *layerUploadController) Cancel() error {
	if err := luc.layerStore.uploadStore.DeleteState(luc.UUID()); err != nil {
		return err
	}

	return luc.Close()
}

func (luc *layerUploadController) Write(p []byte) (int, error) {
	wr, err := luc.file()
	if err != nil {
		return 0, err
	}

	n, err := wr.Write(p)

	// Because we expect the reported offset to be consistent with the storage
	// state, unfortunately, we need to Sync on every call to write.
	if err := wr.Sync(); err != nil {
		// Effectively, ignore the write state if the Sync fails. Report that
		// no bytes were written and seek back to the starting offset.
		offset, seekErr := wr.Seek(luc.Offset(), os.SEEK_SET)
		if seekErr != nil {
			// What do we do here? Quite disasterous.
			luc.reset()

			return 0, fmt.Errorf("multiple errors encounterd after Sync + Seek: %v then %v", err, seekErr)
		}

		if offset != luc.Offset() {
			return 0, fmt.Errorf("unexpected offset after seek")
		}

		return 0, err
	}

	luc.LayerUploadState.Offset += int64(n)

	return n, err
}

func (luc *layerUploadController) Close() error {
	if luc.err != nil {
		return luc.err
	}

	if luc.fp != nil {
		luc.err = luc.fp.Close()
	}

	return luc.err
}

func (luc *layerUploadController) file() (layerFile, error) {
	if luc.fp != nil {
		return luc.fp, nil
	}

	fp, err := luc.uploadStore.Open(luc.UUID())

	if err != nil {
		return nil, err
	}

	// TODO(stevvooe): We may need a more aggressive check here to ensure that
	// the file length is equal to the current offset. We may want to sync the
	// offset before return the layer upload to the client so it can be
	// validated before proceeding with any writes.

	// Seek to the current layer offset for good measure.
	if _, err = fp.Seek(luc.Offset(), os.SEEK_SET); err != nil {
		return nil, err
	}

	luc.fp = fp

	return luc.fp, nil
}

// reset closes and drops the current writer.
func (luc *layerUploadController) reset() {
	if luc.fp != nil {
		luc.fp.Close()
		luc.fp = nil
	}
}

// validateLayer runs several checks on the layer file to ensure its validity.
// This is currently very expensive and relies on fast io and fast seek on the
// local host. If successful, the latest digest is returned, which should be
// used over the passed in value.
func (luc *layerUploadController) validateLayer(fp layerFile, size int64, dgst digest.Digest) (digest.Digest, error) {
	// First, check the incoming tarsum version of the digest.
	version, err := tarsum.GetVersionFromTarsum(dgst.String())
	if err != nil {
		return "", err
	}

	// TODO(stevvooe): Should we push this down into the digest type?
	switch version {
	case tarsum.Version1:
	default:
		// version 0 and dev, for now.
		return "", ErrLayerTarSumVersionUnsupported
	}

	digestVerifier := digest.NewDigestVerifier(dgst)
	lengthVerifier := digest.NewLengthVerifier(size)

	// First, seek to the end of the file, checking the size is as expected.
	end, err := fp.Seek(0, os.SEEK_END)
	if err != nil {
		return "", err
	}

	// Only check size if it is greater than
	if size >= 0 && end != size {
		// Fast path length check.
		return "", ErrLayerInvalidSize{Size: size}
	}

	// Now seek back to start and take care of the digest.
	if _, err := fp.Seek(0, os.SEEK_SET); err != nil {
		return "", err
	}

	tr := io.TeeReader(fp, digestVerifier)

	// Only verify the size if a positive size argument has been passed.
	if size >= 0 {
		tr = io.TeeReader(tr, lengthVerifier)
	}

	// TODO(stevvooe): This is one of the places we need a Digester write
	// sink. Instead, its read driven. This migth be okay.

	// Calculate an updated digest with the latest version.
	dgst, err = digest.FromReader(tr)
	if err != nil {
		return "", err
	}

	if size >= 0 && !lengthVerifier.Verified() {
		return "", ErrLayerInvalidSize{Size: size}
	}

	if !digestVerifier.Verified() {
		return "", ErrLayerInvalidDigest{manifest.FSLayer{BlobSum: dgst}}
	}

	return dgst, nil
}

// writeLayer actually writes the the layer file into its final destination,
// identified by dgst. The layer should be validated before commencing the
// write.
func (luc *layerUploadController) writeLayer(fp layerFile, dgst digest.Digest) (nn int64, err error) {
	blobPath, err := luc.layerStore.pathMapper.path(blobPathSpec{
		digest: dgst,
	})

	if err != nil {
		return 0, err
	}

	// Check for existence
	if _, err := luc.layerStore.driver.Stat(blobPath); err != nil {
		// TODO(stevvooe): This check is kind of problematic and very racy.
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			break // ensure that it doesn't exist.
		default:
			// TODO(stevvooe): This isn't actually an error: the blob store is
			// content addressable and we should just use this to ensure we
			// have it written. Although, we do need to verify that the
			// content that is there is the correct length.
			return 0, err
		}
	}

	// Seek our local layer file back now.
	if _, err := fp.Seek(0, os.SEEK_SET); err != nil {
		// Cleanup?
		return 0, err
	}

	// Okay: we can write the file to the blob store.
	return luc.layerStore.driver.WriteStream(blobPath, 0, fp)
}

// linkLayer links a valid, written layer blob into the registry under the
// named repository for the upload controller.
func (luc *layerUploadController) linkLayer(digest digest.Digest) error {
	layerLinkPath, err := luc.layerStore.pathMapper.path(layerLinkPathSpec{
		name:   luc.Name(),
		digest: digest,
	})

	if err != nil {
		return err
	}

	return luc.layerStore.driver.PutContent(layerLinkPath, []byte(digest))
}

// localFSLayerUploadStore implements a local layerUploadStore. There are some
// complexities around hashsums that make round tripping to the storage
// backend problematic, so we'll store and read locally for now. By GO-beta,
// this should be fully implemented on top of the backend storagedriver.
//
// For now, the directory layout is as follows:
//
// 	/<temp dir>/registry-layer-upload/
// 		<uuid>/
// 			-> state.json
// 			-> data
//
// Each upload, identified by uuid, has its own directory with a state file
// and a data file. The state file has a json representation of the current
// state. The data file is the in-progress upload data.
type localFSLayerUploadStore struct {
	root string
}

func newTemporaryLocalFSLayerUploadStore() (layerUploadStore, error) {
	path, err := ioutil.TempDir("", "registry-layer-upload")

	if err != nil {
		return nil, err
	}

	return &localFSLayerUploadStore{
		root: path,
	}, nil
}

func (llufs *localFSLayerUploadStore) New(name string) (LayerUploadState, error) {
	lus := LayerUploadState{
		Name: name,
		UUID: uuid.New(),
	}

	if err := os.Mkdir(llufs.path(lus.UUID, ""), 0755); err != nil {
		return lus, err
	}

	return lus, nil
}

func (llufs *localFSLayerUploadStore) Open(uuid string) (layerFile, error) {
	fp, err := os.OpenFile(llufs.path(uuid, "data"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)

	if err != nil {
		return nil, err
	}

	return fp, nil
}

func (llufs *localFSLayerUploadStore) GetState(uuid string) (LayerUploadState, error) {
	var lus LayerUploadState

	if _, err := os.Stat(llufs.path(uuid, "")); err != nil {
		if os.IsNotExist(err) {
			return lus, ErrLayerUploadUnknown
		}

		return lus, err
	}
	return lus, nil
}

func (llufs *localFSLayerUploadStore) DeleteState(uuid string) error {
	if err := os.RemoveAll(llufs.path(uuid, "")); err != nil {
		if os.IsNotExist(err) {
			return ErrLayerUploadUnknown
		}

		return err
	}

	return nil
}

func (llufs *localFSLayerUploadStore) path(uuid, file string) string {
	return filepath.Join(llufs.root, uuid, file)
}
