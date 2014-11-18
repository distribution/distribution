package client

import (
	"bytes"
	"crypto/sha1"
	"io"
	"io/ioutil"

	"github.com/docker/docker-registry"

	log "github.com/Sirupsen/logrus"
)

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

	for _, fsLayer := range manifest.FSLayers {
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
			continue
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
