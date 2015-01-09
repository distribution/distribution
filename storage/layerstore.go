package storage

import (
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver"
)

type layerStore struct {
	driver     storagedriver.StorageDriver
	pathMapper *pathMapper
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

	uuid := uuid.New()
	startedAt := time.Now().UTC()

	path, err := ls.pathMapper.path(uploadDataPathSpec{
		name: name,
		uuid: uuid,
	})

	if err != nil {
		return nil, err
	}

	startedAtPath, err := ls.pathMapper.path(uploadStartedAtPathSpec{
		name: name,
		uuid: uuid,
	})

	if err != nil {
		return nil, err
	}

	// Write a startedat file for this upload
	if err := ls.driver.PutContent(startedAtPath, []byte(startedAt.Format(time.RFC3339))); err != nil {
		return nil, err
	}

	return ls.newLayerUpload(name, uuid, path, startedAt)
}

// Resume continues an in progress layer upload, returning the current
// state of the upload.
func (ls *layerStore) Resume(name, uuid string) (LayerUpload, error) {
	startedAtPath, err := ls.pathMapper.path(uploadStartedAtPathSpec{
		name: name,
		uuid: uuid,
	})

	if err != nil {
		return nil, err
	}

	startedAtBytes, err := ls.driver.GetContent(startedAtPath)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, ErrLayerUploadUnknown
		default:
			return nil, err
		}
	}

	startedAt, err := time.Parse(time.RFC3339, string(startedAtBytes))
	if err != nil {
		return nil, err
	}

	path, err := ls.pathMapper.path(uploadDataPathSpec{
		name: name,
		uuid: uuid,
	})

	if err != nil {
		return nil, err
	}

	return ls.newLayerUpload(name, uuid, path, startedAt)
}

// newLayerUpload allocates a new upload controller with the given state.
func (ls *layerStore) newLayerUpload(name, uuid, path string, startedAt time.Time) (LayerUpload, error) {
	fw, err := newFileWriter(ls.driver, path)
	if err != nil {
		return nil, err
	}

	return &layerUploadController{
		layerStore: ls,
		name:       name,
		uuid:       uuid,
		startedAt:  startedAt,
		fileWriter: *fw,
	}, nil
}
