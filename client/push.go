package client

import (
	"bytes"
	"crypto/sha1"
	"io"
	"io/ioutil"

	"github.com/docker/docker-registry"

	log "github.com/Sirupsen/logrus"
)

// simultaneousLayerPushWindow is the size of the parallel layer push window.
// A layer may not be pushed until the layer preceeding it by the length of the
// push window has been successfully pushed.
const simultaneousLayerPushWindow = 4

type pushFunction func(fsLayer registry.FSLayer) error

// Push implements a client push workflow for the image defined by the given
// name and tag pair, using the given ObjectStore for local manifest and layer
// storage
func Push(c Client, objectStore ObjectStore, name, tag string) error {
	manifest, err := objectStore.Manifest(name, tag)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"name":  name,
			"tag":   tag,
		}).Info("No image found")
		return err
	}

	errChans := make([]chan error, len(manifest.FSLayers))
	for i := range manifest.FSLayers {
		errChans[i] = make(chan error)
	}

	// Iterate over each layer in the manifest, simultaneously pushing no more
	// than simultaneousLayerPushWindow layers at a time. If an error is
	// received from a layer push, we abort the push.
	for i := 0; i < len(manifest.FSLayers)+simultaneousLayerPushWindow; i++ {
		dependentLayer := i - simultaneousLayerPushWindow
		if dependentLayer >= 0 {
			err := <-errChans[dependentLayer]
			if err != nil {
				log.WithField("error", err).Warn("Push aborted")
				return err
			}
		}

		if i < len(manifest.FSLayers) {
			go func(i int) {
				errChans[i] <- pushLayer(c, objectStore, name, manifest.FSLayers[i])
			}(i)
		}
	}

	err = c.PutImageManifest(name, tag, manifest)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"manifest": manifest,
		}).Warn("Unable to upload manifest")
		return err
	}

	return nil
}

func pushLayer(c Client, objectStore ObjectStore, name string, fsLayer registry.FSLayer) error {
	log.WithField("layer", fsLayer).Info("Pushing layer")

	layer, err := objectStore.Layer(fsLayer.BlobSum)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to read local layer")
		return err
	}

	layerReader, err := layer.Reader()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to read local layer")
		return err
	}

	location, err := c.InitiateLayerUpload(name, fsLayer.BlobSum)
	if _, ok := err.(*registry.LayerAlreadyExistsError); ok {
		log.WithField("layer", fsLayer).Info("Layer already exists")
		return nil
	}
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to upload layer")
		return err
	}

	layerBuffer := new(bytes.Buffer)
	checksum := sha1.New()
	teeReader := io.TeeReader(layerReader, checksum)

	_, err = io.Copy(layerBuffer, teeReader)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to read local layer")
		return err
	}

	err = c.UploadLayer(location, ioutil.NopCloser(layerBuffer), layerBuffer.Len(),
		&registry.Checksum{HashAlgorithm: "sha1", Sum: string(checksum.Sum(nil))},
	)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to upload layer")
		return err
	}

	return nil
}
