package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker-registry/storagedriver"
)

type layerStore struct {
	driver      storagedriver.StorageDriver
	pathMapper  *pathMapper
	uploadStore layerUploadStore
}

func (ls *layerStore) Exists(tarSum string) (bool, error) {
	// Because this implementation just follows blob links, an existence check
	// is pretty cheap by starting and closing a fetch.
	_, err := ls.Fetch(tarSum)

	if err != nil {
		if err == ErrLayerUnknown {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (ls *layerStore) Fetch(tarSum string) (Layer, error) {
	repos, err := ls.resolveContainingRepositories(tarSum)

	if err != nil {
		// TODO(stevvooe): Unknown tarsum error: need to wrap.
		return nil, err
	}

	// TODO(stevvooe): Access control for layer pulls need to happen here: we
	// have a list of repos that "own" the tarsum that need to be checked
	// against the list of repos to which we have pull access. The argument
	// repos needs to be filtered against that access list.

	name, blobPath, err := ls.resolveBlobPath(repos, tarSum)

	if err != nil {
		// TODO(stevvooe): Map this error correctly, perhaps in the callee.
		return nil, err
	}

	p, err := ls.pathMapper.path(blobPath)
	if err != nil {
		return nil, err
	}

	// Grab the size of the layer file, ensuring that it exists, among other
	// things.
	size, err := ls.driver.CurrentSize(p)

	if err != nil {
		// TODO(stevvooe): Handle blob/path does not exist here.
		// TODO(stevvooe): Get a better understanding of the error cases here
		// that don't stem from unknown path.
		return nil, err
	}

	// Build the layer reader and return to the client.
	layer := &layerReader{
		layerStore: ls,
		path:       p,
		name:       name,
		tarSum:     tarSum,

		// TODO(stevvooe): Storage backend does not support modification time
		// queries yet. Layers "never" change, so just return the zero value.
		createdAt: time.Time{},

		offset: 0,
		size:   int64(size),
	}

	return layer, nil
}

// Upload begins a layer upload, returning a handle. If the layer upload
// is already in progress or the layer has already been uploaded, this
// will return an error.
func (ls *layerStore) Upload(name, tarSum string) (LayerUpload, error) {
	exists, err := ls.Exists(tarSum)
	if err != nil {
		return nil, err
	}

	if exists {
		// TODO(stevvoe): This looks simple now, but we really should only
		// return the layer exists error when the layer exists AND the current
		// client has access to the layer. If the client doesn't have access
		// to the layer, the upload should proceed.
		return nil, ErrLayerExists
	}

	// NOTE(stevvooe): Consider the issues with allowing concurrent upload of
	// the same two layers. Should it be disallowed? For now, we allow both
	// parties to proceed and the the first one uploads the layer.

	lus, err := ls.uploadStore.New(name, tarSum)
	if err != nil {
		return nil, err
	}

	return ls.newLayerUpload(lus), nil
}

// Resume continues an in progress layer upload, returning the current
// state of the upload.
func (ls *layerStore) Resume(name, tarSum, uuid string) (LayerUpload, error) {
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

func (ls *layerStore) resolveContainingRepositories(tarSum string) ([]string, error) {
	// Lookup the layer link in the index by tarsum id.
	layerIndexLinkPath, err := ls.pathMapper.path(layerIndexLinkPathSpec{tarSum: tarSum})
	if err != nil {
		return nil, err
	}

	layerIndexLinkContent, err := ls.driver.GetContent(layerIndexLinkPath)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, ErrLayerUnknown
		default:
			return nil, err
		}
	}

	results := strings.Split(string(layerIndexLinkContent), "\n")

	// clean these up
	for i, result := range results {
		results[i] = strings.TrimSpace(result)
	}

	return results, nil
}

// resolveBlobId lookups up the tarSum in the various repos to find the blob
// link, returning the repo name and blob path spec or an error on failure.
func (ls *layerStore) resolveBlobPath(repos []string, tarSum string) (name string, bps blobPathSpec, err error) {

	for _, repo := range repos {
		pathSpec := layerLinkPathSpec{name: repo, tarSum: tarSum}
		layerLinkPath, err := ls.pathMapper.path(pathSpec)

		if err != nil {
			// TODO(stevvooe): This looks very lazy, may want to collect these
			// errors and report them if we exit this for loop without
			// resolving the blob id.
			logrus.Debugf("error building linkLayerPath (%V): %v", pathSpec, err)
			continue
		}

		layerLinkContent, err := ls.driver.GetContent(layerLinkPath)
		if err != nil {
			logrus.Debugf("error getting layerLink content (%V): %v", pathSpec, err)
			continue
		}

		// Yay! We've resolved our blob id and we're ready to go.
		parts := strings.SplitN(strings.TrimSpace(string(layerLinkContent)), ":", 2)

		if len(parts) != 2 {
			return "", bps, fmt.Errorf("invalid blob reference: %q", string(layerLinkContent))
		}

		name = repo
		bp := blobPathSpec{alg: parts[0], digest: parts[1]}

		return repo, bp, nil
	}

	// TODO(stevvooe): Map this error to repo not found, but it basically
	// means we exited the loop above without finding a blob link.
	return "", bps, fmt.Errorf("unable to resolve blog id for repos=%v and tarSum=%q", repos, tarSum)
}
