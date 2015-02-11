package notifications

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/storage"
)

// ManifestListener describes a set of methods for listening to events related to manifests.
type ManifestListener interface {
	ManifestPushed(repo storage.Repository, sm *manifest.SignedManifest) error
	ManifestPulled(repo storage.Repository, sm *manifest.SignedManifest) error

	// TODO(stevvooe): Please note that delete support is still a little shaky
	// and we'll need to propagate these in the future.

	ManifestDeleted(repo storage.Repository, sm *manifest.SignedManifest) error
}

// LayerListener describes a listener that can respond to layer related events.
type LayerListener interface {
	LayerPushed(repo storage.Repository, layer storage.Layer) error
	LayerPulled(repo storage.Repository, layer storage.Layer) error

	// TODO(stevvooe): Please note that delete support is still a little shaky
	// and we'll need to propagate these in the future.

	LayerDeleted(repo storage.Repository, layer storage.Layer) error
}

// Listener combines all repository events into a single interface.
type Listener interface {
	ManifestListener
	LayerListener
}

type repositoryListener struct {
	storage.Repository
	listener Listener
}

// Listen dispatches events on the repository to the listener.
func Listen(repo storage.Repository, listener Listener) storage.Repository {
	return &repositoryListener{
		Repository: repo,
		listener:   listener,
	}
}

func (rl *repositoryListener) Manifests() storage.ManifestService {
	return &manifestServiceListener{
		ManifestService: rl.Repository.Manifests(),
		parent:          rl,
	}
}

func (rl *repositoryListener) Layers() storage.LayerService {
	return &layerServiceListener{
		LayerService: rl.Repository.Layers(),
		parent:       rl,
	}
}

type manifestServiceListener struct {
	storage.ManifestService
	parent *repositoryListener
}

func (msl *manifestServiceListener) Get(tag string) (*manifest.SignedManifest, error) {
	sm, err := msl.ManifestService.Get(tag)
	if err == nil {
		if err := msl.parent.listener.ManifestPulled(msl.parent.Repository, sm); err != nil {
			logrus.Errorf("error dispatching manifest pull to listener: %v", err)
		}
	}

	return sm, err
}

func (msl *manifestServiceListener) Put(tag string, sm *manifest.SignedManifest) error {
	err := msl.ManifestService.Put(tag, sm)

	if err == nil {
		if err := msl.parent.listener.ManifestPushed(msl.parent.Repository, sm); err != nil {
			logrus.Errorf("error dispatching manifest push to listener: %v", err)
		}
	}

	return err
}

type layerServiceListener struct {
	storage.LayerService
	parent *repositoryListener
}

func (lsl *layerServiceListener) Fetch(dgst digest.Digest) (storage.Layer, error) {
	layer, err := lsl.LayerService.Fetch(dgst)
	if err == nil {
		if err := lsl.parent.listener.LayerPulled(lsl.parent.Repository, layer); err != nil {
			logrus.Errorf("error dispatching layer pull to listener: %v", err)
		}
	}

	return layer, err
}

func (lsl *layerServiceListener) Upload() (storage.LayerUpload, error) {
	lu, err := lsl.LayerService.Upload()
	return lsl.decorateUpload(lu), err
}

func (lsl *layerServiceListener) Resume(uuid string) (storage.LayerUpload, error) {
	lu, err := lsl.LayerService.Resume(uuid)
	return lsl.decorateUpload(lu), err
}

func (lsl *layerServiceListener) decorateUpload(lu storage.LayerUpload) storage.LayerUpload {
	return &layerUploadListener{
		LayerUpload: lu,
		parent:      lsl,
	}
}

type layerUploadListener struct {
	storage.LayerUpload
	parent *layerServiceListener
}

func (lul *layerUploadListener) Finish(dgst digest.Digest) (storage.Layer, error) {
	layer, err := lul.LayerUpload.Finish(dgst)
	if err == nil {
		if err := lul.parent.parent.listener.LayerPushed(lul.parent.parent.Repository, layer); err != nil {
			logrus.Errorf("error dispatching layer push to listener: %v", err)
		}
	}

	return layer, err
}
