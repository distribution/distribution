package storage

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/AdRoll/goamz/cloudfront"
	"github.com/docker/distribution/storagedriver"
)

// cloudFrontLayerHandler provides an simple implementation of layerHandler that
// constructs temporary signed CloudFront URLs from the storagedriver layer URL,
// then issues HTTP Temporary Redirects to this CloudFront content URL.
type cloudFrontLayerHandler struct {
	cloudfront           *cloudfront.CloudFront
	delegateLayerHandler *delegateLayerHandler
	duration             time.Duration
}

var _ LayerHandler = &cloudFrontLayerHandler{}

// newCloudFrontLayerHandler constructs and returns a new CloudFront
// LayerHandler implementation.
// Required options: baseurl, privatekey, keypairid
func newCloudFrontLayerHandler(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (LayerHandler, error) {
	base, ok := options["baseurl"]
	if !ok {
		return nil, fmt.Errorf("No baseurl provided")
	}
	baseURL, ok := base.(string)
	if !ok {
		return nil, fmt.Errorf("baseurl must be a string")
	}
	pk, ok := options["privatekey"]
	if !ok {
		return nil, fmt.Errorf("No privatekey provided")
	}
	pkPath, ok := pk.(string)
	if !ok {
		return nil, fmt.Errorf("privatekey must be a string")
	}
	kpid, ok := options["keypairid"]
	if !ok {
		return nil, fmt.Errorf("No keypairid provided")
	}
	keypairID, ok := kpid.(string)
	if !ok {
		return nil, fmt.Errorf("keypairid must be a string")
	}

	pkBytes, err := ioutil.ReadFile(pkPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read privatekey file: %s", err)
	}

	block, _ := pem.Decode([]byte(pkBytes))
	if block == nil {
		return nil, fmt.Errorf("Failed to decode private key as an rsa private key")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	lh, err := newDelegateLayerHandler(storageDriver, options)
	if err != nil {
		return nil, err
	}
	dlh := lh.(*delegateLayerHandler)

	cf := cloudfront.New(baseURL, privateKey, keypairID)

	duration := 20 * time.Minute
	d, ok := options["duration"]
	if ok {
		switch d := d.(type) {
		case time.Duration:
			duration = d
		case string:
			dur, err := time.ParseDuration(d)
			if err != nil {
				return nil, fmt.Errorf("Invalid duration: %s", err)
			}
			duration = dur
		}
	}

	return &cloudFrontLayerHandler{cloudfront: cf, delegateLayerHandler: dlh, duration: duration}, nil
}

// Resolve returns an http.Handler which can serve the contents of the given
// Layer, or an error if not supported by the storagedriver.
func (lh *cloudFrontLayerHandler) Resolve(layer Layer) (http.Handler, error) {
	layerURLStr, err := lh.delegateLayerHandler.urlFor(layer, nil)
	if err != nil {
		return nil, err
	}

	layerURL, err := url.Parse(layerURLStr)
	if err != nil {
		return nil, err
	}

	cfURL, err := lh.cloudfront.CannedSignedURL(layerURL.Path, "", time.Now().Add(lh.duration))
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cfURL, http.StatusTemporaryRedirect)
	}), nil
}

// init registers the cloudfront layerHandler backend.
func init() {
	RegisterLayerHandler("cloudfront", LayerHandlerInitFunc(newCloudFrontLayerHandler))
}
