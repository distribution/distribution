package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type BlobStorageClient struct {
	client StorageClient
}

// A Container is an entry in ContainerListResponse.
type Container struct {
	Name       string              `xml:"Name"`
	Properties ContainerProperties `xml:"Properties"`
	// TODO (ahmetalpbalkan) Metadata
}

// ContainerProperties contains various properties of a
// container returned from various endpoints like ListContainers.
type ContainerProperties struct {
	LastModified  string `xml:"Last-Modified"`
	Etag          string `xml:"Etag"`
	LeaseStatus   string `xml:"LeaseStatus"`
	LeaseState    string `xml:"LeaseState"`
	LeaseDuration string `xml:"LeaseDuration"`
	// TODO (ahmetalpbalkan) remaining fields
}

// ContainerListResponse contains the response fields from
// ListContainers call. https://msdn.microsoft.com/en-us/library/azure/dd179352.aspx
type ContainerListResponse struct {
	XMLName    xml.Name    `xml:"EnumerationResults"`
	Xmlns      string      `xml:"xmlns,attr"`
	Prefix     string      `xml:"Prefix"`
	Marker     string      `xml:"Marker"`
	NextMarker string      `xml:"NextMarker"`
	MaxResults int64       `xml:"MaxResults"`
	Containers []Container `xml:"Containers>Container"`
}

// A Blob is an entry in BlobListResponse.
type Blob struct {
	Name       string         `xml:"Name"`
	Properties BlobProperties `xml:"Properties"`
	// TODO (ahmetalpbalkan) Metadata
}

// BlobProperties contains various properties of a blob
// returned in various endpoints like ListBlobs or GetBlobProperties.
type BlobProperties struct {
	LastModified          string   `xml:"Last-Modified"`
	Etag                  string   `xml:"Etag"`
	ContentMD5            string   `xml:"Content-MD5"`
	ContentLength         int64    `xml:"Content-Length"`
	ContentType           string   `xml:"Content-Type"`
	ContentEncoding       string   `xml:"Content-Encoding"`
	BlobType              BlobType `xml:"x-ms-blob-blob-type"`
	SequenceNumber        int64    `xml:"x-ms-blob-sequence-number"`
	CopyId                string   `xml:"CopyId"`
	CopyStatus            string   `xml:"CopyStatus"`
	CopySource            string   `xml:"CopySource"`
	CopyProgress          string   `xml:"CopyProgress"`
	CopyCompletionTime    string   `xml:"CopyCompletionTime"`
	CopyStatusDescription string   `xml:"CopyStatusDescription"`
}

// BlobListResponse contains the response fields from
// ListBlobs call. https://msdn.microsoft.com/en-us/library/azure/dd135734.aspx
type BlobListResponse struct {
	XMLName    xml.Name `xml:"EnumerationResults"`
	Xmlns      string   `xml:"xmlns,attr"`
	Prefix     string   `xml:"Prefix"`
	Marker     string   `xml:"Marker"`
	NextMarker string   `xml:"NextMarker"`
	MaxResults int64    `xml:"MaxResults"`
	Blobs      []Blob   `xml:"Blobs>Blob"`
}

// ListContainersParameters defines the set of customizable
// parameters to make a List Containers call. https://msdn.microsoft.com/en-us/library/azure/dd179352.aspx
type ListContainersParameters struct {
	Prefix     string
	Marker     string
	Include    string
	MaxResults uint
	Timeout    uint
}

func (p ListContainersParameters) getParameters() url.Values {
	out := url.Values{}

	if p.Prefix != "" {
		out.Set("prefix", p.Prefix)
	}
	if p.Marker != "" {
		out.Set("marker", p.Marker)
	}
	if p.Include != "" {
		out.Set("include", p.Include)
	}
	if p.MaxResults != 0 {
		out.Set("maxresults", fmt.Sprintf("%v", p.MaxResults))
	}
	if p.Timeout != 0 {
		out.Set("timeout", fmt.Sprintf("%v", p.Timeout))
	}

	return out
}

// ListBlobsParameters defines the set of customizable
// parameters to make a List Blobs call. https://msdn.microsoft.com/en-us/library/azure/dd135734.aspx
type ListBlobsParameters struct {
	Prefix     string
	Delimiter  string
	Marker     string
	Include    string
	MaxResults uint
	Timeout    uint
}

func (p ListBlobsParameters) getParameters() url.Values {
	out := url.Values{}

	if p.Prefix != "" {
		out.Set("prefix", p.Prefix)
	}
	if p.Delimiter != "" {
		out.Set("delimiter", p.Delimiter)
	}
	if p.Marker != "" {
		out.Set("marker", p.Marker)
	}
	if p.Include != "" {
		out.Set("include", p.Include)
	}
	if p.MaxResults != 0 {
		out.Set("maxresults", fmt.Sprintf("%v", p.MaxResults))
	}
	if p.Timeout != 0 {
		out.Set("timeout", fmt.Sprintf("%v", p.Timeout))
	}

	return out
}

// BlobType defines the type of the Azure Blob.
type BlobType string

const (
	BlobTypeBlock BlobType = "BlockBlob"
	BlobTypePage  BlobType = "PageBlob"
)

// PageWriteType defines the type updates that are going to be
// done on the page blob.
type PageWriteType string

const (
	PageWriteTypeUpdate PageWriteType = "update"
	PageWriteTypeClear  PageWriteType = "clear"
)

const (
	blobCopyStatusPending = "pending"
	blobCopyStatusSuccess = "success"
	blobCopyStatusAborted = "aborted"
	blobCopyStatusFailed  = "failed"
)

// BlockListType is used to filter out types of blocks
// in a Get Blocks List call for a block blob. See
// https://msdn.microsoft.com/en-us/library/azure/dd179400.aspx
// for all block types.
type BlockListType string

const (
	BlockListTypeAll         BlockListType = "all"
	BlockListTypeCommitted   BlockListType = "committed"
	BlockListTypeUncommitted BlockListType = "uncommitted"
)

// ContainerAccessType defines the access level to the container
// from a public request. See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx
// and "x-ms-blob-public-access" header.
type ContainerAccessType string

const (
	ContainerAccessTypePrivate   ContainerAccessType = ""
	ContainerAccessTypeBlob      ContainerAccessType = "blob"
	ContainerAccessTypeContainer ContainerAccessType = "container"
)

const (
	MaxBlobBlockSize = 4 * 1024 * 1024
	MaxBlobPageSize  = 4 * 1024 * 1024
)

// BlockStatus defines states a block for a block blob can
// be in.
type BlockStatus string

const (
	BlockStatusUncommitted BlockStatus = "Uncommitted"
	BlockStatusCommitted   BlockStatus = "Committed"
	BlockStatusLatest      BlockStatus = "Latest"
)

// Block is used to create Block entities for Put Block List
// call.
type Block struct {
	Id     string
	Status BlockStatus
}

// BlockListResponse contains the response fields from
// Get Block List call. https://msdn.microsoft.com/en-us/library/azure/dd179400.aspx
type BlockListResponse struct {
	XMLName           xml.Name        `xml:"BlockList"`
	CommittedBlocks   []BlockResponse `xml:"CommittedBlocks>Block"`
	UncommittedBlocks []BlockResponse `xml:"UncommittedBlocks>Block"`
}

// BlockResponse contains the block information returned
// in the GetBlockListCall.
type BlockResponse struct {
	Name string `xml:"Name"`
	Size int64  `xml:"Size"`
}

// GetPageRangesResponse contains the reponse fields from
// Get Page Ranges call. https://msdn.microsoft.com/en-us/library/azure/ee691973.aspx
type GetPageRangesResponse struct {
	XMLName  xml.Name    `xml:"PageList"`
	PageList []PageRange `xml:"PageRange"`
}

// PageRange contains information about a page of a page blob from
// Get Pages Range call. https://msdn.microsoft.com/en-us/library/azure/ee691973.aspx
type PageRange struct {
	Start int64 `xml:"Start"`
	End   int64 `xml:"End"`
}

var (
	ErrNotCreated  = errors.New("storage: operation has returned a successful error code other than 201 Created.")
	ErrNotAccepted = errors.New("storage: operation has returned a successful error code other than 202 Accepted.")

	errBlobCopyAborted    = errors.New("storage: blob copy is aborted")
	errBlobCopyIdMismatch = errors.New("storage: blob copy id is a mismatch")
)

const errUnexpectedStatus = "storage: was expecting status code: %d, got: %d"

// ListContainers returns the list of containers in a storage account along with
// pagination token and other response details. See https://msdn.microsoft.com/en-us/library/azure/dd179352.aspx
func (b BlobStorageClient) ListContainers(params ListContainersParameters) (ContainerListResponse, error) {
	q := mergeParams(params.getParameters(), url.Values{"comp": {"list"}})
	uri := b.client.getEndpoint(blobServiceName, "", q)
	headers := b.client.getStandardHeaders()

	var out ContainerListResponse
	resp, err := b.client.exec("GET", uri, headers, nil)
	if err != nil {
		return out, err
	}

	err = xmlUnmarshal(resp.body, &out)
	return out, err
}

// CreateContainer creates a blob container within the storage account
// with given name and access level. See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx
// Returns error if container already exists.
func (b BlobStorageClient) CreateContainer(name string, access ContainerAccessType) error {
	resp, err := b.createContainer(name, access)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}
	return nil
}

// CreateContainerIfNotExists creates a blob container if it does not exist. Returns
// true if container is newly created or false if container already exists.
func (b BlobStorageClient) CreateContainerIfNotExists(name string, access ContainerAccessType) (bool, error) {
	resp, err := b.createContainer(name, access)
	if resp != nil && (resp.statusCode == http.StatusCreated || resp.statusCode == http.StatusConflict) {
		return resp.statusCode == http.StatusCreated, nil
	}
	return false, err
}

func (b BlobStorageClient) createContainer(name string, access ContainerAccessType) (*storageResponse, error) {
	verb := "PUT"
	uri := b.client.getEndpoint(blobServiceName, pathForContainer(name), url.Values{"restype": {"container"}})

	headers := b.client.getStandardHeaders()
	headers["Content-Length"] = "0"
	if access != "" {
		headers["x-ms-blob-public-access"] = string(access)
	}
	return b.client.exec(verb, uri, headers, nil)
}

// ContainerExists returns true if a container with given name exists
// on the storage account, otherwise returns false.
func (b BlobStorageClient) ContainerExists(name string) (bool, error) {
	verb := "HEAD"
	uri := b.client.getEndpoint(blobServiceName, pathForContainer(name), url.Values{"restype": {"container"}})
	headers := b.client.getStandardHeaders()

	resp, err := b.client.exec(verb, uri, headers, nil)
	if resp != nil && (resp.statusCode == http.StatusOK || resp.statusCode == http.StatusNotFound) {
		return resp.statusCode == http.StatusOK, nil
	}
	return false, err
}

// DeleteContainer deletes the container with given name on the storage
// account. See https://msdn.microsoft.com/en-us/library/azure/dd179408.aspx
// If the container does not exist returns error.
func (b BlobStorageClient) DeleteContainer(name string) error {
	resp, err := b.deleteContainer(name)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusAccepted {
		return ErrNotAccepted
	}
	return nil
}

// DeleteContainer deletes the container with given name on the storage
// account if it exists. See https://msdn.microsoft.com/en-us/library/azure/dd179408.aspx
// Returns true if container is deleted with this call, or false
// if the container did not exist at the time of the Delete Container operation.
func (b BlobStorageClient) DeleteContainerIfExists(name string) (bool, error) {
	resp, err := b.deleteContainer(name)
	if resp != nil && (resp.statusCode == http.StatusAccepted || resp.statusCode == http.StatusNotFound) {
		return resp.statusCode == http.StatusAccepted, nil
	}
	return false, err
}

func (b BlobStorageClient) deleteContainer(name string) (*storageResponse, error) {
	verb := "DELETE"
	uri := b.client.getEndpoint(blobServiceName, pathForContainer(name), url.Values{"restype": {"container"}})

	headers := b.client.getStandardHeaders()
	return b.client.exec(verb, uri, headers, nil)
}

// ListBlobs returns an object that contains list of blobs in the container,
// pagination token and other information in the response of List Blobs call.
// See https://msdn.microsoft.com/en-us/library/azure/dd135734.aspx
func (b BlobStorageClient) ListBlobs(container string, params ListBlobsParameters) (BlobListResponse, error) {
	q := mergeParams(params.getParameters(), url.Values{
		"restype": {"container"},
		"comp":    {"list"}})
	uri := b.client.getEndpoint(blobServiceName, pathForContainer(container), q)
	headers := b.client.getStandardHeaders()

	var out BlobListResponse
	resp, err := b.client.exec("GET", uri, headers, nil)
	if err != nil {
		return out, err
	}

	err = xmlUnmarshal(resp.body, &out)
	return out, err
}

// BlobExists returns true if a blob with given name exists on the
// specified container of the storage account.
func (b BlobStorageClient) BlobExists(container, name string) (bool, error) {
	verb := "HEAD"
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})

	headers := b.client.getStandardHeaders()
	resp, err := b.client.exec(verb, uri, headers, nil)
	if resp != nil && (resp.statusCode == http.StatusOK || resp.statusCode == http.StatusNotFound) {
		return resp.statusCode == http.StatusOK, nil
	}
	return false, err
}

// GetBlobUrl gets the canonical URL to the blob with the specified
// name in the specified container. This method does not create a
// publicly accessible URL if the blob or container is private and this
// method does not check if the blob exists.
func (b BlobStorageClient) GetBlobUrl(container, name string) string {
	if container == "" {
		container = "$root"
	}
	return b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})
}

// GetBlob downloads a blob to a stream. See https://msdn.microsoft.com/en-us/library/azure/dd179440.aspx
func (b BlobStorageClient) GetBlob(container, name string) (io.ReadCloser, error) {
	resp, err := b.getBlobRange(container, name, "")
	if err != nil {
		return nil, err
	}

	if resp.statusCode != http.StatusOK {
		return nil, fmt.Errorf(errUnexpectedStatus, http.StatusOK, resp.statusCode)
	}
	return resp.body, nil
}

// GetBlobRange reads the specified range of a blob to a stream.
// The bytesRange string must be in a format like "0-", "10-100"
// as defined in HTTP 1.1 spec. See https://msdn.microsoft.com/en-us/library/azure/dd179440.aspx
func (b BlobStorageClient) GetBlobRange(container, name, bytesRange string) (io.ReadCloser, error) {
	resp, err := b.getBlobRange(container, name, bytesRange)
	if err != nil {
		return nil, err
	}

	if resp.statusCode != http.StatusPartialContent {
		return nil, fmt.Errorf(errUnexpectedStatus, http.StatusPartialContent, resp.statusCode)
	}
	return resp.body, nil
}

func (b BlobStorageClient) getBlobRange(container, name, bytesRange string) (*storageResponse, error) {
	verb := "GET"
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})

	headers := b.client.getStandardHeaders()
	if bytesRange != "" {
		headers["Range"] = fmt.Sprintf("bytes=%s", bytesRange)
	}
	resp, err := b.client.exec(verb, uri, headers, nil)
	if err != nil {
		return nil, err
	}
	return resp, err
}

// GetBlobProperties provides various information about the specified
// blob. See https://msdn.microsoft.com/en-us/library/azure/dd179394.aspx
func (b BlobStorageClient) GetBlobProperties(container, name string) (*BlobProperties, error) {
	verb := "HEAD"
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})

	headers := b.client.getStandardHeaders()
	resp, err := b.client.exec(verb, uri, headers, nil)
	if err != nil {
		return nil, err
	}

	if resp.statusCode != http.StatusOK {
		return nil, fmt.Errorf(errUnexpectedStatus, http.StatusOK, resp.statusCode)
	}

	var contentLength int64
	contentLengthStr := resp.headers.Get("Content-Length")
	if contentLengthStr != "" {
		contentLength, err = strconv.ParseInt(contentLengthStr, 0, 64)
		if err != nil {
			return nil, err
		}
	}

	var sequenceNum int64
	sequenceNumStr := resp.headers.Get("x-ms-blob-sequence-number")
	if sequenceNumStr != "" {
		sequenceNum, err = strconv.ParseInt(sequenceNumStr, 0, 64)
		if err != nil {
			return nil, err
		}
	}

	return &BlobProperties{
		LastModified:          resp.headers.Get("Last-Modified"),
		Etag:                  resp.headers.Get("Etag"),
		ContentMD5:            resp.headers.Get("Content-MD5"),
		ContentLength:         contentLength,
		ContentEncoding:       resp.headers.Get("Content-Encoding"),
		SequenceNumber:        sequenceNum,
		CopyCompletionTime:    resp.headers.Get("x-ms-copy-completion-time"),
		CopyStatusDescription: resp.headers.Get("x-ms-copy-status-description"),
		CopyId:                resp.headers.Get("x-ms-copy-id"),
		CopyProgress:          resp.headers.Get("x-ms-copy-progress"),
		CopySource:            resp.headers.Get("x-ms-copy-source"),
		CopyStatus:            resp.headers.Get("x-ms-copy-status"),
		BlobType:              BlobType(resp.headers.Get("x-ms-blob-type")),
	}, nil
}

// CreateBlockBlob initializes an empty block blob with no blocks.
// See https://msdn.microsoft.com/en-us/library/azure/dd179451.aspx
func (b BlobStorageClient) CreateBlockBlob(container, name string) error {
	path := fmt.Sprintf("%s/%s", container, name)
	uri := b.client.getEndpoint(blobServiceName, path, url.Values{})
	headers := b.client.getStandardHeaders()
	headers["x-ms-blob-type"] = string(BlobTypeBlock)
	headers["Content-Length"] = fmt.Sprintf("%v", 0)

	resp, err := b.client.exec("PUT", uri, headers, nil)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}
	return nil
}

// PutBlockBlob uploads given stream into a block blob by splitting
// data stream into chunks and uploading as blocks. Commits the block
// list at the end. This is a helper method built on top of PutBlock
// and PutBlockList methods with sequential block ID counting logic.
func (b BlobStorageClient) PutBlockBlob(container, name string, blob io.Reader) error { // TODO (ahmetalpbalkan) consider ReadCloser and closing
	return b.putBlockBlob(container, name, blob, MaxBlobBlockSize)
}

func (b BlobStorageClient) putBlockBlob(container, name string, blob io.Reader, chunkSize int) error {
	if chunkSize <= 0 || chunkSize > MaxBlobBlockSize {
		chunkSize = MaxBlobBlockSize
	}

	chunk := make([]byte, chunkSize)
	n, err := blob.Read(chunk)
	if err != nil && err != io.EOF {
		return err
	}

	if err == io.EOF {
		// Fits into one block
		return b.putSingleBlockBlob(container, name, chunk[:n])
	} else {
		// Does not fit into one block. Upload block by block then commit the block list
		blockList := []Block{}

		// Put blocks
		for blockNum := 0; ; blockNum++ {
			id := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%011d", blockNum)))
			data := chunk[:n]
			err = b.PutBlock(container, name, id, data)
			if err != nil {
				return err
			}
			blockList = append(blockList, Block{id, BlockStatusLatest})

			// Read next block
			n, err = blob.Read(chunk)
			if err != nil && err != io.EOF {
				return err
			}
			if err == io.EOF {
				break
			}
		}

		// Commit block list
		return b.PutBlockList(container, name, blockList)
	}
}

func (b BlobStorageClient) putSingleBlockBlob(container, name string, chunk []byte) error {
	if len(chunk) > MaxBlobBlockSize {
		return fmt.Errorf("storage: provided chunk (%d bytes) cannot fit into single-block blob (max %d bytes)", len(chunk), MaxBlobBlockSize)
	}

	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})
	headers := b.client.getStandardHeaders()
	headers["x-ms-blob-type"] = string(BlobTypeBlock)
	headers["Content-Length"] = fmt.Sprintf("%v", len(chunk))

	resp, err := b.client.exec("PUT", uri, headers, bytes.NewReader(chunk))
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}

	return nil
}

// PutBlock saves the given data chunk to the specified block blob with
// given ID. See https://msdn.microsoft.com/en-us/library/azure/dd135726.aspx
func (b BlobStorageClient) PutBlock(container, name, blockId string, chunk []byte) error {
	return b.PutBlockWithLength(container, name, blockId, uint64(len(chunk)), bytes.NewReader(chunk))
}

// PutBlockWithLength saves the given data stream of exactly specified size to the block blob
// with given ID. See https://msdn.microsoft.com/en-us/library/azure/dd135726.aspx
// It is an alternative to PutBlocks where data comes as stream but the length is
// known in advance.
func (b BlobStorageClient) PutBlockWithLength(container, name, blockId string, size uint64, blob io.Reader) error {
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{"comp": {"block"}, "blockid": {blockId}})
	headers := b.client.getStandardHeaders()
	headers["x-ms-blob-type"] = string(BlobTypeBlock)
	headers["Content-Length"] = fmt.Sprintf("%v", size)

	resp, err := b.client.exec("PUT", uri, headers, blob)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}

	return nil
}

// PutBlockList saves list of blocks to the specified block blob. See
// https://msdn.microsoft.com/en-us/library/azure/dd179467.aspx
func (b BlobStorageClient) PutBlockList(container, name string, blocks []Block) error {
	blockListXml := prepareBlockListRequest(blocks)

	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{"comp": {"blocklist"}})
	headers := b.client.getStandardHeaders()
	headers["Content-Length"] = fmt.Sprintf("%v", len(blockListXml))

	resp, err := b.client.exec("PUT", uri, headers, strings.NewReader(blockListXml))
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}
	return nil
}

// GetBlockList retrieves list of blocks in the specified block blob. See
// https://msdn.microsoft.com/en-us/library/azure/dd179400.aspx
func (b BlobStorageClient) GetBlockList(container, name string, blockType BlockListType) (BlockListResponse, error) {
	params := url.Values{"comp": {"blocklist"}, "blocklisttype": {string(blockType)}}
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), params)
	headers := b.client.getStandardHeaders()

	var out BlockListResponse
	resp, err := b.client.exec("GET", uri, headers, nil)
	if err != nil {
		return out, err
	}

	err = xmlUnmarshal(resp.body, &out)
	return out, err
}

// PutPageBlob initializes an empty page blob with specified name and maximum
// size in bytes (size must be aligned to a 512-byte boundary). A page blob must
// be created using this method before writing pages.
// See https://msdn.microsoft.com/en-us/library/azure/dd179451.aspx
func (b BlobStorageClient) PutPageBlob(container, name string, size int64) error {
	path := fmt.Sprintf("%s/%s", container, name)
	uri := b.client.getEndpoint(blobServiceName, path, url.Values{})
	headers := b.client.getStandardHeaders()
	headers["x-ms-blob-type"] = string(BlobTypePage)
	headers["x-ms-blob-content-length"] = fmt.Sprintf("%v", size)
	headers["Content-Length"] = fmt.Sprintf("%v", 0)

	resp, err := b.client.exec("PUT", uri, headers, nil)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}
	return nil
}

// PutPage writes a range of pages to a page blob or clears the given range.
// In case of 'clear' writes, given chunk is discarded. Ranges must be aligned
// with 512-byte boundaries and chunk must be of size multiplies by 512.
// See https://msdn.microsoft.com/en-us/library/ee691975.aspx
func (b BlobStorageClient) PutPage(container, name string, startByte, endByte int64, writeType PageWriteType, chunk []byte) error {
	path := fmt.Sprintf("%s/%s", container, name)
	uri := b.client.getEndpoint(blobServiceName, path, url.Values{"comp": {"page"}})
	headers := b.client.getStandardHeaders()
	headers["x-ms-blob-type"] = string(BlobTypePage)
	headers["x-ms-page-write"] = string(writeType)
	headers["x-ms-range"] = fmt.Sprintf("bytes=%v-%v", startByte, endByte)

	var contentLength int64
	var data io.Reader
	if writeType == PageWriteTypeClear {
		contentLength = 0
		data = bytes.NewReader([]byte{})
	} else {
		contentLength = int64(len(chunk))
		data = bytes.NewReader(chunk)
	}
	headers["Content-Length"] = fmt.Sprintf("%v", contentLength)

	resp, err := b.client.exec("PUT", uri, headers, data)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusCreated {
		return ErrNotCreated
	}
	return nil
}

// GetPageRanges returns the list of valid page ranges for a page blob.
// See https://msdn.microsoft.com/en-us/library/azure/ee691973.aspx
func (b BlobStorageClient) GetPageRanges(container, name string) (GetPageRangesResponse, error) {
	path := fmt.Sprintf("%s/%s", container, name)
	uri := b.client.getEndpoint(blobServiceName, path, url.Values{"comp": {"pagelist"}})
	headers := b.client.getStandardHeaders()

	var out GetPageRangesResponse
	resp, err := b.client.exec("GET", uri, headers, nil)
	if err != nil {
		return out, err
	}

	if resp.statusCode != http.StatusOK {
		return out, fmt.Errorf(errUnexpectedStatus, http.StatusOK, resp.statusCode)
	}

	err = xmlUnmarshal(resp.body, &out)
	return out, err
}

// CopyBlob starts a blob copy operation and waits for the operation to complete.
// sourceBlob parameter must be a canonical URL to the blob (can be obtained using
// GetBlobURL method.) There is no SLA on blob copy and therefore this helper
// method works faster on smaller files. See https://msdn.microsoft.com/en-us/library/azure/dd894037.aspx
func (b BlobStorageClient) CopyBlob(container, name, sourceBlob string) error {
	copyId, err := b.startBlobCopy(container, name, sourceBlob)
	if err != nil {
		return err
	}

	return b.waitForBlobCopy(container, name, copyId)
}

func (b BlobStorageClient) startBlobCopy(container, name, sourceBlob string) (string, error) {
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})

	headers := b.client.getStandardHeaders()
	headers["Content-Length"] = "0"
	headers["x-ms-copy-source"] = sourceBlob

	resp, err := b.client.exec("PUT", uri, headers, nil)
	if err != nil {
		return "", err
	}
	if resp.statusCode != http.StatusAccepted && resp.statusCode != http.StatusCreated {
		return "", fmt.Errorf(errUnexpectedStatus, []int{http.StatusAccepted, http.StatusCreated}, resp.statusCode)
	}

	copyId := resp.headers.Get("x-ms-copy-id")
	if copyId == "" {
		return "", errors.New("Got empty copy id header")
	}
	return copyId, nil
}

func (b BlobStorageClient) waitForBlobCopy(container, name, copyId string) error {
	for {
		props, err := b.GetBlobProperties(container, name)
		if err != nil {
			return err
		}

		if props.CopyId != copyId {
			return errBlobCopyIdMismatch
		}

		switch props.CopyStatus {
		case blobCopyStatusSuccess:
			return nil
		case blobCopyStatusPending:
			continue
		case blobCopyStatusAborted:
			return errBlobCopyAborted
		case blobCopyStatusFailed:
			return fmt.Errorf("storage: blob copy failed. Id=%s Description=%s", props.CopyId, props.CopyStatusDescription)
		default:
			return fmt.Errorf("storage: unhandled blob copy status: '%s'", props.CopyStatus)
		}
	}
}

// DeleteBlob deletes the given blob from the specified container.
// If the blob does not exists at the time of the Delete Blob operation, it
// returns error. See https://msdn.microsoft.com/en-us/library/azure/dd179413.aspx
func (b BlobStorageClient) DeleteBlob(container, name string) error {
	resp, err := b.deleteBlob(container, name)
	if err != nil {
		return err
	}
	if resp.statusCode != http.StatusAccepted {
		return ErrNotAccepted
	}
	return nil
}

// DeleteBlobIfExists deletes the given blob from the specified container
// If the blob is deleted with this call, returns true. Otherwise returns
// false. See https://msdn.microsoft.com/en-us/library/azure/dd179413.aspx
func (b BlobStorageClient) DeleteBlobIfExists(container, name string) (bool, error) {
	resp, err := b.deleteBlob(container, name)
	if resp != nil && (resp.statusCode == http.StatusAccepted || resp.statusCode == http.StatusNotFound) {
		return resp.statusCode == http.StatusAccepted, nil
	}
	return false, err
}

func (b BlobStorageClient) deleteBlob(container, name string) (*storageResponse, error) {
	verb := "DELETE"
	uri := b.client.getEndpoint(blobServiceName, pathForBlob(container, name), url.Values{})
	headers := b.client.getStandardHeaders()

	return b.client.exec(verb, uri, headers, nil)
}

// helper method to construct the path to a container given its name
func pathForContainer(name string) string {
	return fmt.Sprintf("/%s", name)
}

// helper method to construct the path to a blob given its container and blob name
func pathForBlob(container, name string) string {
	return fmt.Sprintf("/%s/%s", container, name)
}

// GetBlobSASURI creates an URL to the specified blob which contains the Shared Access Signature
// with specified permissions and expiration time. See https://msdn.microsoft.com/en-us/library/azure/ee395415.aspx
func (b BlobStorageClient) GetBlobSASURI(container, name string, expiry time.Time, permissions string) (string, error) {
	var (
		signedPermissions = permissions
		blobUrl           = b.GetBlobUrl(container, name)
	)
	canonicalizedResource, err := b.client.buildCanonicalizedResource(blobUrl)
	if err != nil {
		return "", err
	}
	signedExpiry := expiry.Format(time.RFC3339)
	signedResource := "b"

	stringToSign, err := blobSASStringToSign(b.client.apiVersion, canonicalizedResource, signedExpiry, signedPermissions)
	if err != nil {
		return "", err
	}

	sig := b.client.computeHmac256(stringToSign)
	sasParams := url.Values{
		"sv":  {b.client.apiVersion},
		"se":  {signedExpiry},
		"sr":  {signedResource},
		"sp":  {signedPermissions},
		"sig": {sig},
	}

	sasUrl, err := url.Parse(blobUrl)
	if err != nil {
		return "", err
	}
	sasUrl.RawQuery = sasParams.Encode()
	return sasUrl.String(), nil
}

func blobSASStringToSign(signedVersion, canonicalizedResource, signedExpiry, signedPermissions string) (string, error) {
	var signedStart, signedIdentifier, rscc, rscd, rsce, rscl, rsct string

	// reference: http://msdn.microsoft.com/en-us/library/azure/dn140255.aspx
	if signedVersion >= "2013-08-15" {
		return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s", signedPermissions, signedStart, signedExpiry, canonicalizedResource, signedIdentifier, signedVersion, rscc, rscd, rsce, rscl, rsct), nil
	} else {
		return "", errors.New("storage: not implemented SAS for versions earlier than 2013-08-15")
	}
}
