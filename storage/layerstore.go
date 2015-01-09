package storage

import (
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver"
)

type layerStore struct {
	driver      storagedriver.StorageDriver
	pathMapper  *pathMapper
	uploadStore layerUploadStore
}

func (ls *layerStore) Exists(name string, digest digest.Digest) (bool, error) {
	// Because this implementation just follows blob links, an existence check
	// is pretty cheap by starting and closing a fetch.
	_, err := ls.Fetch(name, digest)

	if err != nil {
		switch err.(type) {
		case ErrUnknownLayer:
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (ls *layerStore) Fetch(name string, digest digest.Digest) (Layer, error) {
	blobPath, err := resolveBlobPath(ls.driver, ls.pathMapper, name, digest)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError, *storagedriver.PathNotFoundError:
			return nil, ErrUnknownLayer{manifest.FSLayer{BlobSum: digest}}
		default:
			return nil, err
		}
	}

	fr, err := newFileReader(ls.driver, blobPath)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError, *storagedriver.PathNotFoundError:
			return nil, ErrUnknownLayer{manifest.FSLayer{BlobSum: digest}}
		default:
			return nil, err
		}
	}

	return &layerReader{
		fileReader: *fr,
		name:       name,
		digest:     digest,
	}, nil
}

// Upload begins a layer upload, returning a handle. If the layer upload
// is already in progress or the layer has already been uploaded, this
// will return an error.
func (ls *layerStore) Upload(name string) (LayerUpload, error) {

	// NOTE(stevvooe): Consider the issues with allowing concurrent upload of
	// the same two layers. Should it be disallowed? For now, we allow both
	// parties to proceed and the the first one uploads the layer.

	lus, err := ls.uploadStore.New(name)
	if err != nil {
		return nil, err
	}

	return ls.newLayerUpload(lus), nil
}

// Resume continues an in progress layer upload, returning the current
// state of the upload.
func (ls *layerStore) Resume(lus LayerUploadState) (LayerUpload, error) {
	_, err := ls.uploadStore.GetState(lus.UUID)

	if err != nil {
		return nil, err
	}

	return ls.newLayerUpload(lus), nil
}

// newLayerUpload allocates a new upload controller with the given state.
func (ls *layerStore) newLayerUpload(lus LayerUploadState) LayerUpload {
	return &layerUploadController{
		LayerUploadState: lus,
		layerStore:       ls,
		uploadStore:      ls.uploadStore,
	}
}
