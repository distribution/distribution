package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	DefaultBaseUrl    = "core.windows.net"
	DefaultApiVersion = "2014-02-14"
	defaultUseHttps   = true

	blobServiceName  = "blob"
	tableServiceName = "table"
	queueServiceName = "queue"
)

// StorageClient is the object that needs to be constructed
// to perform operations on the storage account.
type StorageClient struct {
	accountName string
	accountKey  []byte
	useHttps    bool
	baseUrl     string
	apiVersion  string
}

type storageResponse struct {
	statusCode int
	headers    http.Header
	body       io.ReadCloser
}

// StorageServiceError contains fields of the error response from
// Azure Storage Service REST API. See https://msdn.microsoft.com/en-us/library/azure/dd179382.aspx
// Some fields might be specific to certain calls.
type StorageServiceError struct {
	Code                      string `xml:"Code"`
	Message                   string `xml:"Message"`
	AuthenticationErrorDetail string `xml:"AuthenticationErrorDetail"`
	QueryParameterName        string `xml:"QueryParameterName"`
	QueryParameterValue       string `xml:"QueryParameterValue"`
	Reason                    string `xml:"Reason"`
	StatusCode                int
	RequestId                 string
}

// NewBasicClient constructs a StorageClient with given storage service name
// and key.
func NewBasicClient(accountName, accountKey string) (StorageClient, error) {
	return NewClient(accountName, accountKey, DefaultBaseUrl, DefaultApiVersion, defaultUseHttps)
}

// NewClient constructs a StorageClient. This should be used if the caller
// wants to specify whether to use HTTPS, a specific REST API version or a
// custom storage endpoint than Azure Public Cloud.
func NewClient(accountName, accountKey, blobServiceBaseUrl, apiVersion string, useHttps bool) (StorageClient, error) {
	var c StorageClient
	if accountName == "" {
		return c, fmt.Errorf("azure: account name required")
	} else if accountKey == "" {
		return c, fmt.Errorf("azure: account key required")
	} else if blobServiceBaseUrl == "" {
		return c, fmt.Errorf("azure: base storage service url required")
	}

	key, err := base64.StdEncoding.DecodeString(accountKey)
	if err != nil {
		return c, err
	}

	return StorageClient{
		accountName: accountName,
		accountKey:  key,
		useHttps:    useHttps,
		baseUrl:     blobServiceBaseUrl,
		apiVersion:  apiVersion,
	}, nil
}

func (c StorageClient) getBaseUrl(service string) string {
	scheme := "http"
	if c.useHttps {
		scheme = "https"
	}

	host := fmt.Sprintf("%s.%s.%s", c.accountName, service, c.baseUrl)

	u := &url.URL{
		Scheme: scheme,
		Host:   host}
	return u.String()
}

func (c StorageClient) getEndpoint(service, path string, params url.Values) string {
	u, err := url.Parse(c.getBaseUrl(service))
	if err != nil {
		// really should not be happening
		panic(err)
	}

	if path == "" {
		path = "/" // API doesn't accept path segments not starting with '/'
	}

	u.Path = path
	u.RawQuery = params.Encode()
	return u.String()
}

// GetBlobService returns a BlobStorageClient which can operate on the
// blob service of the storage account.
func (c StorageClient) GetBlobService() *BlobStorageClient {
	return &BlobStorageClient{c}
}

func (c StorageClient) createAuthorizationHeader(canonicalizedString string) string {
	signature := c.computeHmac256(canonicalizedString)
	return fmt.Sprintf("%s %s:%s", "SharedKey", c.accountName, signature)
}

func (c StorageClient) getAuthorizationHeader(verb, url string, headers map[string]string) (string, error) {
	canonicalizedResource, err := c.buildCanonicalizedResource(url)
	if err != nil {
		return "", err
	}

	canonicalizedString := c.buildCanonicalizedString(verb, headers, canonicalizedResource)
	return c.createAuthorizationHeader(canonicalizedString), nil
}

func (c StorageClient) getStandardHeaders() map[string]string {
	return map[string]string{
		"x-ms-version": c.apiVersion,
		"x-ms-date":    currentTimeRfc1123Formatted(),
	}
}

func (c StorageClient) buildCanonicalizedHeader(headers map[string]string) string {
	cm := make(map[string]string)

	for k, v := range headers {
		headerName := strings.TrimSpace(strings.ToLower(k))
		match, _ := regexp.MatchString("x-ms-", headerName)
		if match {
			cm[headerName] = v
		}
	}

	if len(cm) == 0 {
		return ""
	}

	keys := make([]string, 0, len(cm))
	for key := range cm {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	ch := ""

	for i, key := range keys {
		if i == len(keys)-1 {
			ch += fmt.Sprintf("%s:%s", key, cm[key])
		} else {
			ch += fmt.Sprintf("%s:%s\n", key, cm[key])
		}
	}
	return ch
}

func (c StorageClient) buildCanonicalizedResource(uri string) (string, error) {
	errMsg := "buildCanonicalizedResource error: %s"
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf(errMsg, err.Error())
	}

	cr := "/" + c.accountName
	if len(u.Path) > 0 {
		cr += u.Path
	}

	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf(errMsg, err.Error())
	}

	if len(params) > 0 {
		cr += "\n"
		keys := make([]string, 0, len(params))
		for key := range params {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for i, key := range keys {
			if len(params[key]) > 1 {
				sort.Strings(params[key])
			}

			if i == len(keys)-1 {
				cr += fmt.Sprintf("%s:%s", key, strings.Join(params[key], ","))
			} else {
				cr += fmt.Sprintf("%s:%s\n", key, strings.Join(params[key], ","))
			}
		}
	}
	return cr, nil
}

func (c StorageClient) buildCanonicalizedString(verb string, headers map[string]string, canonicalizedResource string) string {
	canonicalizedString := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
		verb,
		headers["Content-Encoding"],
		headers["Content-Language"],
		headers["Content-Length"],
		headers["Content-MD5"],
		headers["Content-Type"],
		headers["Date"],
		headers["If-Modified-Singe"],
		headers["If-Match"],
		headers["If-None-Match"],
		headers["If-Unmodified-Singe"],
		headers["Range"],
		c.buildCanonicalizedHeader(headers),
		canonicalizedResource)

	return canonicalizedString
}

func (c StorageClient) exec(verb, url string, headers map[string]string, body io.Reader) (*storageResponse, error) {
	authHeader, err := c.getAuthorizationHeader(verb, url, headers)
	if err != nil {
		return nil, err
	}
	headers["Authorization"] = authHeader

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(verb, url, body)
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	httpClient := http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	statusCode := resp.StatusCode
	if statusCode >= 400 && statusCode <= 505 {
		var respBody []byte
		respBody, err = readResponseBody(resp)
		if err != nil {
			return nil, err
		}

		if len(respBody) == 0 {
			// no error in response body
			err = fmt.Errorf("storage: service returned without a response body (%s).", resp.Status)
		} else {
			// response contains storage service error object, unmarshal
			storageErr, errIn := serviceErrFromXml(respBody, resp.StatusCode, resp.Header.Get("x-ms-request-id"))
			if err != nil { // error unmarshaling the error response
				err = errIn
			}
			err = storageErr
		}
		return &storageResponse{
			statusCode: resp.StatusCode,
			headers:    resp.Header,
			body:       ioutil.NopCloser(bytes.NewReader(respBody)), /* restore the body */
		}, err
	}

	return &storageResponse{
		statusCode: resp.StatusCode,
		headers:    resp.Header,
		body:       resp.Body}, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	out, err := ioutil.ReadAll(resp.Body)
	if err == io.EOF {
		err = nil
	}
	return out, err
}

func serviceErrFromXml(body []byte, statusCode int, requestId string) (StorageServiceError, error) {
	var storageErr StorageServiceError
	if err := xml.Unmarshal(body, &storageErr); err != nil {
		return storageErr, err
	}
	storageErr.StatusCode = statusCode
	storageErr.RequestId = requestId
	return storageErr, nil
}

func (e StorageServiceError) Error() string {
	return fmt.Sprintf("storage: remote server returned error. StatusCode=%d, ErrorCode=%s, ErrorMessage=%s, RequestId=%s", e.StatusCode, e.Code, e.Message, e.RequestId)
}
