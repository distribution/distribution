package client

import (
	"fmt"
	"io"

	"github.com/docker/docker-registry/storage"

	log "github.com/Sirupsen/logrus"
)

// simultaneousLayerPullWindow is the size of the parallel layer pull window.
// A layer may not be pulled until the layer preceeding it by the length of the
// pull window has been successfully pulled.
const simultaneousLayerPullWindow = 4

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

	errChans := make([]chan error, len(manifest.FSLayers))
	for i := range manifest.FSLayers {
		errChans[i] = make(chan error)
	}

	// To avoid leak of goroutines we must notify
	// pullLayer goroutines about a cancelation,
	// otherwise they will lock forever.
	cancelCh := make(chan struct{})

	// Iterate over each layer in the manifest, simultaneously pulling no more
	// than simultaneousLayerPullWindow layers at a time. If an error is
	// received from a layer pull, we abort the push.
	for i := 0; i < len(manifest.FSLayers)+simultaneousLayerPullWindow; i++ {
		dependentLayer := i - simultaneousLayerPullWindow
		if dependentLayer >= 0 {
			err := <-errChans[dependentLayer]
			if err != nil {
				log.WithField("error", err).Warn("Pull aborted")
				close(cancelCh)
				return err
			}
		}

		if i < len(manifest.FSLayers) {
			go func(i int) {
				select {
				case errChans[i] <- pullLayer(c, objectStore, name, manifest.FSLayers[i]):
				case <-cancelCh: // no chance to recv until cancelCh's closed
				}
			}(i)
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

func pullLayer(c Client, objectStore ObjectStore, name string, fsLayer storage.FSLayer) error {
	log.WithField("layer", fsLayer).Info("Pulling layer")

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
		return nil
	}
	if err == ErrLayerLocked {
		log.WithField("layer", fsLayer).Info("Layer download in progress, waiting")
		layer.Wait()
		return nil
	}
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"layer": fsLayer,
		}).Warn("Unable to write local layer")
		return err
	}
	defer writer.Close()

	layerReader, length, err := c.GetBlob(name, fsLayer.BlobSum, 0)
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
		return fmt.Errorf(
			"Wrote incorrect number of bytes for layer %v. Expected %d, Wrote %d",
			fsLayer, length, copied,
		)
	}
	return nil
}
