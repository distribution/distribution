package storage

import (
	"github.com/docker/docker-registry/digest"
	"github.com/docker/docker-registry/storagedriver"
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
	blobPath, err := ls.resolveBlobPath(name, digest)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError, *storagedriver.PathNotFoundError:
			return nil, ErrUnknownLayer{FSLayer{BlobSum: digest}}
		default:
			return nil, err
		}
	}

	fr, err := newFileReader(ls.driver, blobPath)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError, *storagedriver.PathNotFoundError:
			return nil, ErrUnknownLayer{FSLayer{BlobSum: digest}}
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
func (ls *layerStore) Resume(uuid string) (LayerUpload, error) {
	lus, err := ls.uploadStore.GetState(uuid)

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

// resolveBlobId looks up the blob location in the repositories from a
// layer/blob link file, returning blob path or an error on failure.
func (ls *layerStore) resolveBlobPath(name string, dgst digest.Digest) (string, error) {
	pathSpec := layerLinkPathSpec{name: name, digest: dgst}
	layerLinkPath, err := ls.pathMapper.path(pathSpec)

	if err != nil {
		return "", err
	}

	layerLinkContent, err := ls.driver.GetContent(layerLinkPath)
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

	return ls.pathMapper.path(bp)
}
