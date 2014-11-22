package storage

import (
	"encoding/json"
	"fmt"

	"github.com/docker/libtrust"

	"github.com/docker/docker-registry/storagedriver"
)

type manifestStore struct {
	driver       storagedriver.StorageDriver
	pathMapper   *pathMapper
	layerService LayerService
}

var _ ManifestService = &manifestStore{}

func (ms *manifestStore) Exists(name, tag string) (bool, error) {
	p, err := ms.path(name, tag)
	if err != nil {
		return false, err
	}

	size, err := ms.driver.CurrentSize(p)
	if err != nil {
		return false, err
	}

	if size == 0 {
		return false, nil
	}

	return true, nil
}

func (ms *manifestStore) Get(name, tag string) (*SignedManifest, error) {
	p, err := ms.path(name, tag)
	if err != nil {
		return nil, err
	}

	content, err := ms.driver.GetContent(p)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError, *storagedriver.PathNotFoundError:
			return nil, ErrManifestUnknown
		default:
			return nil, err
		}
	}

	var manifest SignedManifest

	if err := json.Unmarshal(content, &manifest); err != nil {
		// TODO(stevvooe): Corrupted manifest error?
		return nil, err
	}

	// TODO(stevvooe): Verify the manifest here?

	return &manifest, nil
}

func (ms *manifestStore) Put(name, tag string, manifest *SignedManifest) error {
	p, err := ms.path(name, tag)
	if err != nil {
		return err
	}

	if err := ms.verifyManifest(name, tag, manifest); err != nil {
		return err
	}

	// TODO(stevvooe): Should we get manifest first?

	return ms.driver.PutContent(p, manifest.Raw)
}

func (ms *manifestStore) Delete(name, tag string) error {
	panic("not implemented")
}

func (ms *manifestStore) path(name, tag string) (string, error) {
	return ms.pathMapper.path(manifestPathSpec{
		name: name,
		tag:  tag,
	})
}

func (ms *manifestStore) verifyManifest(name, tag string, manifest *SignedManifest) error {
	if manifest.Name != name {
		return fmt.Errorf("name does not match manifest name")
	}

	if manifest.Tag != tag {
		return fmt.Errorf("tag does not match manifest tag")
	}

	var errs []error

	for _, fsLayer := range manifest.FSLayers {
		exists, err := ms.layerService.Exists(name, fsLayer.BlobSum)
		if err != nil {
			// TODO(stevvooe): Need to store information about missing blob.
			errs = append(errs, err)
		}

		if !exists {
			errs = append(errs, fmt.Errorf("missing layer %v", fsLayer.BlobSum))
		}
	}

	if len(errs) != 0 {
		// TODO(stevvooe): These need to be recoverable by a caller.
		return fmt.Errorf("missing layers: %v", errs)
	}

	js, err := libtrust.ParsePrettySignature(manifest.Raw, "signatures")
	if err != nil {
		return err
	}

	_, err = js.Verify() // These pubkeys need to be checked.
	if err != nil {
		return err
	}

	// TODO(sday): Pubkey checks need to go here. This where things get fancy.
	// Perhaps, an injected service would reduce coupling here.

	return nil
}
