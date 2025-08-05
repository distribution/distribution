package bunnystorage

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/sha256-simd"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

type Client struct {
	*fasthttp.Client
	logger   resty.Logger
	password string
	baseUrl  url.URL
}

// Initialize a new bunnystorage-go client with default settings. Endpoint format is https://<region endpoint>/<Storage Zone Name> e.g. https://la.storage.bunnycdn.com/mystoragezone/
func NewClient(endpoint url.URL, password string) Client {
	return Client{
		&fasthttp.Client{
			ReadTimeout:  time.Second * 30,
			WriteTimeout: time.Second * 30,
		},
		logrus.New(),
		password,
		endpoint,
	}
}

// Add a custom logger. The logger has to implement the resty.Logger interface
func (c *Client) WithLogger(l resty.Logger) *Client {
	c.logger = l
	return c
}

func (c *Client) prepareRequest(path string) *fasthttp.Request {
	req := fasthttp.AcquireRequest()
	reqUrl := c.baseUrl.JoinPath(path)
	req.Header.Add("AccessKey", c.password)
	req.SetRequestURI(reqUrl.String())
	return req
}

// Uploads a file to the relative path. generateChecksum controls if a checksum gets generated and attached to the upload request. Returns an error.
func (c *Client) Upload(path string, content []byte, generateChecksum bool) error {

	req := c.prepareRequest(path)
	defer fasthttp.ReleaseRequest(req)

	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.SetMethod(fasthttp.MethodPut)
	req.SetBody(content)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	if generateChecksum {
		checksum := sha256.New()
		_, err := checksum.Write(content)
		if err != nil {
			return err
		}
		hex_checksum := hex.EncodeToString(checksum.Sum(nil))
		req.Header.Add("Checksum", hex_checksum)
	}

	err := c.Do(req, resp)
	c.logger.Debugf("Put Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("Put Request Failed: %v", err)
		return err
	}
	if isHTTPError(resp.StatusCode()) {
		return errors.New(string(resp.Header.StatusMessage()))
	}
	return nil
}

// Downloads a file from a path.
func (c *Client) Download(path string) ([]byte, error) {
	req := c.prepareRequest(path)
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(fasthttp.MethodGet)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err := c.Do(req, resp)
	c.logger.Debugf("Get Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("Get Request Failed: %v", err)
		return nil, err
	}
	if isHTTPError(resp.StatusCode()) {
		return nil, errors.New(string(resp.Header.StatusMessage()))
	}
	respBody := make([]byte, len(resp.Body()))
	copy(respBody, resp.Body())
	return respBody, nil
}

// Downloads a byte range of a file. Uses the semantics for HTTP range requests. If you want to avoid passing buffers directly for performance, use DownloadPartialWithReaderCloser
//
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Range_requests
func (c *Client) DownloadPartial(path string, rangeStart int64, rangeEnd int64) ([]byte, error) {
	req := c.prepareRequest(path)
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err := c.Do(req, resp)
	c.logger.Debugf("Get Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("Get Request Failed: %v", err)
		return nil, err
	}
	if isHTTPError(resp.StatusCode()) {
		return nil, errors.New(string(resp.Header.StatusMessage()))
	}
	respBody := make([]byte, len(resp.Body()))
	copy(respBody, resp.Body())
	return respBody, nil
}

// Delete a file or a directory. If the path to delete is a directory, set the isPath flag to true
func (c *Client) Delete(path string, isPath bool) error {
	if isPath {
		path += "/" // The trailing slash is required to delete a directory
	}

	req := c.prepareRequest(path)
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(fasthttp.MethodDelete)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err := c.Do(req, resp)
	c.logger.Debugf("Delete Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("Delete Request Failed: %v", err)
		return err
	}
	if isHTTPError(resp.StatusCode()) {
		return errors.New(string(resp.Header.StatusMessage()))
	}
	return nil
}

// Lists files from a directory.
func (c *Client) List(path string) ([]Object, error) {
	req := c.prepareRequest(path + "/") // The trailing slash is neccessary, since without it the API will treat the requested directory as a file and returns an empty list
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Add("Accept", "application/json")

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := c.Do(req, resp)
	c.logger.Debugf("List Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("List Request Failed: %v", err)
		return nil, err
	}
	if isHTTPError(resp.StatusCode()) {
		return nil, errors.New(string(resp.Header.StatusMessage()))
	}

	objectList := make([]Object, 0)

	err = json.Unmarshal(resp.Body(), &objectList)
	if err != nil {
		return nil, err
	}
	return objectList, nil
}

// Describes an Object. EXPERIMENTAL. The official Java SDK uses it, but the DESCRIBE HTTP method used is not officially documented.
func (c *Client) Describe(path string) (Object, error) {
	object := Object{}

	req := c.prepareRequest(path)
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod("DESCRIBE")
	req.Header.Add("Accept", "application/json")

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := c.Do(req, resp)
	c.logger.Debugf("Describe Request Response: %v", resp)

	if err != nil {
		c.logger.Errorf("Describe Request Failed: %v", err)
		return object, err
	}
	if isHTTPError(resp.StatusCode()) {
		return object, errors.New(string(resp.Header.StatusMessage()))
	}

	err = json.Unmarshal(resp.Body(), &object)
	if err != nil {
		return object, err
	}
	return object, nil
}

func isHTTPError(c int) bool {
	return c > 399
}
