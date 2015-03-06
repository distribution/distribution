package notifications

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
)

// ManifestListener describes a set of methods for listening to events related to manifests.
type ManifestListener interface {
	ManifestPushed(repo distribution.Repository, sm *manifest.SignedManifest) error
	ManifestPulled(repo distribution.Repository, sm *manifest.SignedManifest) error

	// TODO(stevvooe): Please note that delete support is still a little shaky
	// and we'll need to propagate these in the future.

	ManifestDeleted(repo distribution.Repository, sm *manifest.SignedManifest) error
}

// LayerListener describes a listener that can respond to layer related events.
type LayerListener interface {
	LayerPushed(repo distribution.Repository, layer distribution.Layer) error
	LayerPulled(repo distribution.Repository, layer distribution.Layer) error

	// TODO(stevvooe): Please note that delete support is still a little shaky
	// and we'll need to propagate these in the future.

	LayerDeleted(repo distribution.Repository, layer distribution.Layer) error
}

// Listener combines all repository events into a single interface.
type Listener interface {
	ManifestListener
	LayerListener
}

type repositoryListener struct {
	distribution.Repository
	listener Listener
}

// Listen dispatches events on the repository to the listener.
func Listen(repo distribution.Repository, listener Listener) distribution.Repository {
	return &repositoryListener{
		Repository: repo,
		listener:   listener,
	}
}

func (rl *repositoryListener) Manifests() distribution.ManifestService {
	return &manifestServiceListener{
		ManifestService: rl.Repository.Manifests(),
		parent:          rl,
	}
}

func (rl *repositoryListener) Layers() distribution.LayerService {
	return &layerServiceListener{
		LayerService: rl.Repository.Layers(),
		parent:       rl,
	}
}

type manifestServiceListener struct {
	distribution.ManifestService
	parent *repositoryListener
}

func (msl *manifestServiceListener) Get(dgst digest.Digest) (*manifest.SignedManifest, error) {
	sm, err := msl.ManifestService.Get(dgst)
	if err == nil {
		if err := msl.parent.listener.ManifestPulled(msl.parent.Repository, sm); err != nil {
			logrus.Errorf("error dispatching manifest pull to listener: %v", err)
		}
	}

	return sm, err
}

func (msl *manifestServiceListener) Put(sm *manifest.SignedManifest) error {
	err := msl.ManifestService.Put(sm)

	if err == nil {
		if err := msl.parent.listener.ManifestPushed(msl.parent.Repository, sm); err != nil {
			logrus.Errorf("error dispatching manifest push to listener: %v", err)
		}
	}

	return err
}

func (msl *manifestServiceListener) GetByTag(tag string) (*manifest.SignedManifest, error) {
	sm, err := msl.ManifestService.GetByTag(tag)
	if err == nil {
		if err := msl.parent.listener.ManifestPulled(msl.parent.Repository, sm); err != nil {
			logrus.Errorf("error dispatching manifest pull to listener: %v", err)
		}
	}

	return sm, err
}

type layerServiceListener struct {
	distribution.LayerService
	parent *repositoryListener
}

func (lsl *layerServiceListener) Fetch(dgst digest.Digest) (distribution.Layer, error) {
	layer, err := lsl.LayerService.Fetch(dgst)
	if err == nil {
		if err := lsl.parent.listener.LayerPulled(lsl.parent.Repository, layer); err != nil {
			logrus.Errorf("error dispatching layer pull to listener: %v", err)
		}
	}

	return layer, err
}

func (lsl *layerServiceListener) Upload() (distribution.LayerUpload, error) {
	lu, err := lsl.LayerService.Upload()
	return lsl.decorateUpload(lu), err
}

func (lsl *layerServiceListener) Resume(uuid string) (distribution.LayerUpload, error) {
	lu, err := lsl.LayerService.Resume(uuid)
	return lsl.decorateUpload(lu), err
}

func (lsl *layerServiceListener) decorateUpload(lu distribution.LayerUpload) distribution.LayerUpload {
	return &layerUploadListener{
		LayerUpload: lu,
		parent:      lsl,
	}
}

type layerUploadListener struct {
	distribution.LayerUpload
	parent *layerServiceListener
}

func (lul *layerUploadListener) Finish(dgst digest.Digest) (distribution.Layer, error) {
	layer, err := lul.LayerUpload.Finish(dgst)
	if err == nil {
		if err := lul.parent.parent.listener.LayerPushed(lul.parent.parent.Repository, layer); err != nil {
			logrus.Errorf("error dispatching layer push to listener: %v", err)
		}
	}

	return layer, err
}
