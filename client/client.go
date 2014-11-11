package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/docker/docker-registry"
)

// Client implements the client interface to the registry http api
type Client interface {
	// GetImageManifest returns an image manifest for the image at the given
	// name, tag pair
	GetImageManifest(name, tag string) (*registry.ImageManifest, error)

	// PutImageManifest uploads an image manifest for the image at the given
	// name, tag pair
	PutImageManifest(name, tag string, imageManifest *registry.ImageManifest) error

	// DeleteImage removes the image at the given name, tag pair
	DeleteImage(name, tag string) error

	// ListImageTags returns a list of all image tags with the given repository
	// name
	ListImageTags(name string) ([]string, error)

	// GetImageLayer returns the image layer at the given name, tarsum pair in
	// the form of an io.ReadCloser with the length of this layer
	// A nonzero byteOffset can be provided to receive a partial layer beginning
	// at the given offset
	GetImageLayer(name, tarsum string, byteOffset int) (io.ReadCloser, int, error)

	// InitiateLayerUpload starts an image upload for the given name, tarsum
	// pair and returns a unique location url to use for other layer upload
	// methods
	// Returns a *registry.LayerAlreadyExistsError if the layer already exists
	// on the registry
	InitiateLayerUpload(name, tarsum string) (string, error)

	// GetLayerUploadStatus returns the byte offset and length of the layer at
	// the given upload location
	GetLayerUploadStatus(location string) (int, int, error)

	// UploadLayer uploads a full image layer to the registry
	UploadLayer(location string, layer io.ReadCloser, length int, checksum *registry.Checksum) error

	// UploadLayerChunk uploads a layer chunk with a given length and startByte
	// to the registry
	// FinishChunkedLayerUpload must be called to finalize this upload
	UploadLayerChunk(location string, layerChunk io.ReadCloser, length, startByte int) error

	// FinishChunkedLayerUpload completes a chunked layer upload at a given
	// location
	FinishChunkedLayerUpload(location string, length int, checksum *registry.Checksum) error

	// CancelLayerUpload deletes all content at the unfinished layer upload
	// location and invalidates any future calls to this layer upload
	CancelLayerUpload(location string) error
}

// New returns a new Client which operates against a registry with the
// given base endpoint
// This endpoint should not include /v2/ or any part of the url after this
func New(endpoint string) Client {
	return &clientImpl{endpoint}
}

// clientImpl is the default implementation of the Client interface
type clientImpl struct {
	Endpoint string
}

// TODO(bbland): use consistent route generation between server and client

func (r *clientImpl) GetImageManifest(name, tag string) (*registry.ImageManifest, error) {
	response, err := http.Get(r.imageManifestUrl(name, tag))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusOK:
		break
	case response.StatusCode == http.StatusNotFound:
		return nil, &registry.ImageManifestNotFoundError{name, tag}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return nil, err
		}
		return nil, errors
	default:
		return nil, &registry.UnexpectedHttpStatusError{response.Status}
	}

	decoder := json.NewDecoder(response.Body)

	manifest := new(registry.ImageManifest)
	err = decoder.Decode(manifest)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (r *clientImpl) PutImageManifest(name, tag string, manifest *registry.ImageManifest) error {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	putRequest, err := http.NewRequest("PUT",
		r.imageManifestUrl(name, tag), bytes.NewReader(manifestBytes))
	if err != nil {
		return err
	}

	response, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusOK:
		return nil
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) DeleteImage(name, tag string) error {
	deleteRequest, err := http.NewRequest("DELETE",
		r.imageManifestUrl(name, tag), nil)
	if err != nil {
		return err
	}

	response, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusNoContent:
		break
	case response.StatusCode == http.StatusNotFound:
		return &registry.ImageManifestNotFoundError{name, tag}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}

	return nil
}

func (r *clientImpl) ListImageTags(name string) ([]string, error) {
	response, err := http.Get(fmt.Sprintf("%s/v2/%s/tags", r.Endpoint, name))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusOK:
		break
	case response.StatusCode == http.StatusNotFound:
		return nil, &registry.RepositoryNotFoundError{name}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return nil, err
		}
		return nil, errors
	default:
		return nil, &registry.UnexpectedHttpStatusError{response.Status}
	}

	tags := struct {
		Tags []string `json:"tags"`
	}{}

	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&tags)
	if err != nil {
		return nil, err
	}

	return tags.Tags, nil
}

func (r *clientImpl) GetImageLayer(name, tarsum string, byteOffset int) (io.ReadCloser, int, error) {
	getRequest, err := http.NewRequest("GET",
		fmt.Sprintf("%s/v2/%s/layer/%s", r.Endpoint, name, tarsum), nil)
	if err != nil {
		return nil, 0, err
	}

	getRequest.Header.Add("Range", fmt.Sprintf("%d-", byteOffset))
	response, err := http.DefaultClient.Do(getRequest)
	if err != nil {
		return nil, 0, err
	}

	if response.StatusCode == http.StatusNotFound {
		return nil, 0, &registry.LayerNotFoundError{name, tarsum}
	}
	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusOK:
		lengthHeader := response.Header.Get("Content-Length")
		length, err := strconv.ParseInt(lengthHeader, 10, 0)
		if err != nil {
			return nil, 0, err
		}
		return response.Body, int(length), nil
	case response.StatusCode == http.StatusNotFound:
		response.Body.Close()
		return nil, 0, &registry.LayerNotFoundError{name, tarsum}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return nil, 0, err
		}
		return nil, 0, errors
	default:
		response.Body.Close()
		return nil, 0, &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) InitiateLayerUpload(name, tarsum string) (string, error) {
	postRequest, err := http.NewRequest("POST",
		fmt.Sprintf("%s/v2/%s/layer/%s/upload", r.Endpoint, name, tarsum), nil)
	if err != nil {
		return "", err
	}

	response, err := http.DefaultClient.Do(postRequest)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusAccepted:
		return response.Header.Get("Location"), nil
	case response.StatusCode == http.StatusNotModified:
		return "", &registry.LayerAlreadyExistsError{name, tarsum}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return "", err
		}
		return "", errors
	default:
		return "", &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) GetLayerUploadStatus(location string) (int, int, error) {
	response, err := http.Get(fmt.Sprintf("%s%s", r.Endpoint, location))
	if err != nil {
		return 0, 0, err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusNoContent:
		return parseRangeHeader(response.Header.Get("Range"))
	case response.StatusCode == http.StatusNotFound:
		return 0, 0, &registry.LayerUploadNotFoundError{location}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return 0, 0, err
		}
		return 0, 0, errors
	default:
		return 0, 0, &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) UploadLayer(location string, layer io.ReadCloser, length int, checksum *registry.Checksum) error {
	defer layer.Close()

	putRequest, err := http.NewRequest("PUT",
		fmt.Sprintf("%s%s", r.Endpoint, location), layer)
	if err != nil {
		return err
	}

	queryValues := new(url.Values)
	queryValues.Set("length", fmt.Sprint(length))
	queryValues.Set(checksum.HashAlgorithm, checksum.Sum)
	putRequest.URL.RawQuery = queryValues.Encode()

	putRequest.Header.Set("Content-Type", "application/octet-stream")
	putRequest.Header.Set("Content-Length", fmt.Sprint(length))

	response, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusCreated:
		return nil
	case response.StatusCode == http.StatusNotFound:
		return &registry.LayerUploadNotFoundError{location}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) UploadLayerChunk(location string, layerChunk io.ReadCloser, length, startByte int) error {
	defer layerChunk.Close()

	putRequest, err := http.NewRequest("PUT",
		fmt.Sprintf("%s%s", r.Endpoint, location), layerChunk)
	if err != nil {
		return err
	}

	endByte := startByte + length

	putRequest.Header.Set("Content-Type", "application/octet-stream")
	putRequest.Header.Set("Content-Length", fmt.Sprint(length))
	putRequest.Header.Set("Content-Range",
		fmt.Sprintf("%d-%d/%d", startByte, endByte, endByte))

	response, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusAccepted:
		return nil
	case response.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		lastValidRange, layerSize, err := parseRangeHeader(response.Header.Get("Range"))
		if err != nil {
			return err
		}
		return &registry.LayerUploadInvalidRangeError{location, lastValidRange, layerSize}
	case response.StatusCode == http.StatusNotFound:
		return &registry.LayerUploadNotFoundError{location}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) FinishChunkedLayerUpload(location string, length int, checksum *registry.Checksum) error {
	putRequest, err := http.NewRequest("PUT",
		fmt.Sprintf("%s%s", r.Endpoint, location), nil)
	if err != nil {
		return err
	}

	queryValues := new(url.Values)
	queryValues.Set("length", fmt.Sprint(length))
	queryValues.Set(checksum.HashAlgorithm, checksum.Sum)
	putRequest.URL.RawQuery = queryValues.Encode()

	putRequest.Header.Set("Content-Type", "application/octet-stream")
	putRequest.Header.Set("Content-Length", "0")
	putRequest.Header.Set("Content-Range",
		fmt.Sprintf("%d-%d/%d", length, length, length))

	response, err := http.DefaultClient.Do(putRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusCreated:
		return nil
	case response.StatusCode == http.StatusNotFound:
		return &registry.LayerUploadNotFoundError{location}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}
}

func (r *clientImpl) CancelLayerUpload(location string) error {
	deleteRequest, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s%s", r.Endpoint, location), nil)
	if err != nil {
		return err
	}

	response, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// TODO(bbland): handle other status codes, like 5xx errors
	switch {
	case response.StatusCode == http.StatusNoContent:
		return nil
	case response.StatusCode == http.StatusNotFound:
		return &registry.LayerUploadNotFoundError{location}
	case response.StatusCode >= 400 && response.StatusCode < 500:
		errors := new(registry.Errors)
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&errors)
		if err != nil {
			return err
		}
		return errors
	default:
		return &registry.UnexpectedHttpStatusError{response.Status}
	}
}

// imageManifestUrl is a helper method for returning the full url to an image
// manifest
func (r *clientImpl) imageManifestUrl(name, tag string) string {
	return fmt.Sprintf("%s/v2/%s/image/%s", r.Endpoint, name, tag)
}

// parseRangeHeader parses out the offset and length from a returned Range
// header
func parseRangeHeader(byteRangeHeader string) (int, int, error) {
	r := regexp.MustCompile("bytes=0-(\\d+)/(\\d+)")
	submatches := r.FindStringSubmatch(byteRangeHeader)
	offset, err := strconv.ParseInt(submatches[1], 10, 0)
	if err != nil {
		return 0, 0, err
	}
	length, err := strconv.ParseInt(submatches[2], 10, 0)
	if err != nil {
		return 0, 0, err
	}
	return int(offset), int(length), nil
}
