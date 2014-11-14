package client

import (
	"fmt"
	"io"

	log "github.com/Sirupsen/logrus"
)

// Pull implements a client pull workflow for the image defined by the given
// name and tag pair, using the given ObjectStore for local manifest and layer
// storage
func Pull(c Client, objectStore ObjectStore, name, tag string) error {
	manifest, err := c.GetImageManifest(name, tag)
	if err != nil {
		return err
	}
	log.WithField("manifest", manifest).Info("Pulled manifest")

	if len(manifest.FSLayers) != len(manifest.History) {
		return fmt.Errorf("Length of history not equal to number of layers")
	}
	if len(manifest.FSLayers) == 0 {
		return fmt.Errorf("Image has no layers")
	}

	for _, fsLayer := range manifest.FSLayers {
		layer, err := objectStore.Layer(fsLayer.BlobSum)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"layer": fsLayer,
			}).Warn("Unable to write local layer")
			return err
		}

		writer, err := layer.Writer()
		if err == ErrLayerAlreadyExists {
			log.WithField("layer", fsLayer).Info("Layer already exists")
			continue
		}
		if err == ErrLayerLocked {
			log.WithField("layer", fsLayer).Info("Layer download in progress, waiting")
			layer.Wait()
			continue
		}
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"layer": fsLayer,
			}).Warn("Unable to write local layer")
			return err
		}
		defer writer.Close()

		layerReader, length, err := c.GetImageLayer(name, fsLayer.BlobSum, 0)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"layer": fsLayer,
			}).Warn("Unable to download layer")
			return err
		}
		defer layerReader.Close()

		copied, err := io.Copy(writer, layerReader)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"layer": fsLayer,
			}).Warn("Unable to download layer")
			return err
		}
		if copied != int64(length) {
			log.WithFields(log.Fields{
				"expected": length,
				"written":  copied,
				"layer":    fsLayer,
			}).Warn("Wrote incorrect number of bytes for layer")
		}
	}

	err = objectStore.WriteManifest(name, tag, manifest)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"manifest": manifest,
		}).Warn("Unable to write image manifest")
		return err
	}

	return nil
}
