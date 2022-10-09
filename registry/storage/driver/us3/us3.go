package us3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	ufsdk "github.com/ufilesdk-dev/ufile-gosdk"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

const (
	driverName = "us3"
	apiName    = "api.ucloud.cn"

	listMax  = 1000 // listMax is the largest amount of object you can request from US3 in a ListObject call
	limit    = 100  // limit is the largest amount of uploadIds you can request from US3 in a GetMultiUploadId call
	maxParts = 1000 // maxParts is the largest amount of parts you can request from US3 in a GetMultiUploadPart call

	defaultBlkSize = 4 << 20 // 4MB
)

var BlkSize = 0 // the BlkSize that InitiateMultipartUpload returns

// DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	PublicKey       string
	PrivateKey      string
	Api             string
	Bucket          string
	Regin           string
	Endpoint        string
	CustomDomain    string
	VerifyUploadMD5 bool
	RootDirectory   string
}

func init() {
	factory.Register(driverName, &us3DriverFactory{})
}

// us3DriverFactory implements the factory.StorageDriverFactory interface
type us3DriverFactory struct{}

// Implement factory.StorageDriverFactory interface
func (factory *us3DriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Req             *ufsdk.UFileRequest
	PublicKey       string
	PrivateKey      string
	Api             string
	Bucket          string
	Endpoint        string
	VerifyUploadMD5 bool
	RootDirectory   string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by UCloud US3
// Objects are stored at absolute keys in the provided bucket.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - PublicKey
// - PrivateKey
// - Bucket
// - Regin
// - Endpoint
// Optional parameters:
// - CustomDomain
// - Api
// - VerifyUploadMD5
// - RootDirectory
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	publicKey, ok := parameters["PublicKey"]
	if !ok {
		return nil, fmt.Errorf("No PublicKey parameter provided")
	}
	privateKey, ok := parameters["PrivateKey"]
	if !ok {
		return nil, fmt.Errorf("No PrivateKey parameter provided")
	}
	api, ok := parameters["Api"]
	if !ok {
		api = apiName
	}
	bucket, ok := parameters["Bucket"]
	if !ok {
		return nil, fmt.Errorf("No Bucket parameter provided")
	}
	regin, ok := parameters["Regin"]
	if !ok {
		return nil, fmt.Errorf("No Regin parameter provided")
	}
	endpoint, ok := parameters["Endpoint"]
	if !ok {
		return nil, fmt.Errorf("No Endpoint parameter provided")
	}
	// when you point a CustomDomain like http://xxxxxx.com, the request url will be customDomain
	// like http://xxxxxx.com/xxxx or the request url will be http://<Bucket>.<Endpoint>/xxxx
	customDomain, ok := parameters["CustomDomain"]
	if !ok {
		customDomain = ""
	}
	verifyUploadMD5 := false
	verifyUploadMD5Bool, ok := parameters["VerifyUploadMD5"]
	if ok {
		verifyUploadMD5, ok = verifyUploadMD5Bool.(bool)
		if !ok {
			return nil, fmt.Errorf("VerifyUploadMD5 parameter is not a type of 'bool'")
		}
	}
	rootDirectory, ok := parameters["RootDirectory"]
	if !ok {
		rootDirectory = ""
	}

	param := DriverParameters{
		PublicKey:       fmt.Sprint(publicKey),
		PrivateKey:      fmt.Sprint(privateKey),
		Api:             fmt.Sprint(api),
		Bucket:          fmt.Sprint(bucket),
		Regin:           fmt.Sprint(regin),
		Endpoint:        fmt.Sprint(endpoint),
		CustomDomain:    fmt.Sprint(customDomain),
		VerifyUploadMD5: verifyUploadMD5,
		RootDirectory:   fmt.Sprint(rootDirectory),
	}
	return New(param)
}

// New constructs a new Driver with the given UCloud credentials, region, encryption flag, and
// bucketName
func New(params DriverParameters) (*Driver, error) {
	config := &ufsdk.Config{
		PublicKey:       params.PublicKey,
		PrivateKey:      params.PrivateKey,
		BucketHost:      params.Api,
		BucketName:      params.Bucket,
		FileHost:        params.Endpoint,
		VerifyUploadMD5: params.VerifyUploadMD5,
		Endpoint:        params.CustomDomain,
	}

	req, err := ufsdk.NewFileRequest(config, nil)
	if err != nil {
		return nil, err
	}

	// Validate that the given credentials have at least read permissions in the
	// given bucket scope.
	if _, err := req.ListObjects(strings.TrimRight(params.RootDirectory, "/"), "", "", 1); err != nil {
		return nil, err
	}

	d := &driver{
		Req:             req,
		PublicKey:       params.PublicKey,
		PrivateKey:      params.PrivateKey,
		Api:             params.Api,
		Bucket:          params.Bucket,
		Endpoint:        params.Endpoint,
		VerifyUploadMD5: params.VerifyUploadMD5,
		RootDirectory:   params.RootDirectory,
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

// Implement the storagedriver.StorageDriver interface
func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	data, err := d.getContent(d.us3Path(path), 0)
	if err != nil {
		return nil, parseError(path, d.Req.ParseError())
	}
	return data, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	if len(contents) >= defaultBlkSize { // multi parts streaming upload
		return d.Req.IOMutipartAsyncUpload(bytes.NewReader(contents), d.us3Path(path), d.getContentType())
	} else { // ordinary streaming upload
		return d.Req.IOPut(bytes.NewReader(contents), d.us3Path(path), d.getContentType())
	}
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	respBody, err := d.Req.DownloadFileRetRespBody(d.us3Path(path), offset)
	if err != nil {
		err = d.Req.ParseError()
		if us3Err, ok := err.(*ufsdk.Error); ok && us3Err.StatusCode == http.StatusRequestedRangeNotSatisfiable && (us3Err.ErrMsg == "invalid range" || us3Err.RetCode == 0) {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}
		return nil, parseError(path, err)
	}
	return respBody, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	key := d.us3Path(path)
	if !append {
		state, err := d.Req.InitiateMultipartUpload(key, d.getContentType())
		if err != nil {
			return nil, err
		}
		BlkSize = state.BlkSize
		return d.newWriter(key, state, nil), nil
	}
	list, err := d.Req.GetMultiUploadId(key, "", "", limit)
	if err != nil {
		return nil, parseError(path, d.Req.ParseError())
	}
	for _, dataSet := range list.DataSet {
		if key != dataSet.FileName {
			continue
		}
		parts, err := d.Req.GetMultiUploadPart(dataSet.UploadId, maxParts, 0)
		if err != nil {
			return nil, parseError(path, d.Req.ParseError())
		}
		if err != nil {
			return nil, err
		}
		state := new(ufsdk.MultipartState)
		state.GenerateMultipartState(BlkSize, dataSet.UploadId, d.getContentType(), key, parts)
		return d.newWriter(key, state, parts), nil
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	list, err := d.Req.ListObjects(d.us3Path(path), "", "", 1) // 返回包含 prefix 的所有文件，包括文件夹
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(list.Contents) == 1 {
		if list.Contents[0].Key != d.us3Path(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			size, err := strconv.ParseInt(list.Contents[0].Size, 10, 64)
			if err != nil {
				return nil, err
			}
			fi.Size = size
			timestamp := time.Unix(int64(list.Contents[0].LastModified), 0)
			fi.ModTime = timestamp
		}
	} else if len(list.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && opath[len(path)-1] != '/' {
		path = path + "/"
	}

	prefix := ""
	if d.us3Path("") == "" {
		prefix = "/"
	}

	us3Path := d.us3Path(path)
	listResponse, err := d.Req.ListObjects(us3Path, "", "/", listMax)
	if err != nil {
		return nil, parseError(path, d.Req.ParseError())
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range listResponse.Contents {
			files = append(files, strings.Replace(key.Key, d.us3Path(""), prefix, 1))
		}

		for _, commonPrefix := range listResponse.CommonPrefixes {
			commonPrefix := commonPrefix.Prefix
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.us3Path(""), prefix, 1))
		}

		if listResponse.IsTruncated {
			listResponse, err = d.Req.ListObjects(us3Path, listResponse.NextMarker, "/", listMax)
			if err != nil {
				return nil, parseError(path, d.Req.ParseError())
			}
		} else {
			break
		}
	}

	// This is to cover for the cases when the first key equal to us3Path.
	if len(files) > 0 && files[0] == strings.Replace(us3Path, d.us3Path(""), prefix, 1) {
		files = files[1:]
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			// Treat empty response as missing directory, since we don't actually have directories in us3.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	err := d.Req.Copy(d.us3Path(destPath), d.Bucket, d.us3Path(sourcePath))
	if err != nil {
		return parseError(sourcePath, d.Req.ParseError())
	}
	return d.Delete(ctx, sourcePath)
}

// path = /ABC, target = /ABCabc, return false
// path = /ABC, target = /ABCabc/XYZ, return false
// path = /ABC, target = /ABC/abc, return true
// path = /ABC, target = /ABC, return true
func isSubPath(path, target string) (bool, error) {
	if len(path) > len(target) {
		return false, fmt.Errorf("Something error, path should be prefix of target. While path is %s, target is %s\n", path, target)
	} else if len(path) == len(target) || target[len(path)] == '/' {
		return true, nil
	}
	return false, nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	_, statErr := d.Stat(ctx, path)
	if err, ok := statErr.(storagedriver.PathNotFoundError); ok {
		return err
	}
	prefix := d.us3Path(path)
	marker := ""
	for {
		list, err := d.Req.ListObjects(prefix, marker, "", listMax)
		if err != nil {
			return parseError(path, d.Req.ParseError())
		}
		for _, object := range list.Contents {
			ok, err := isSubPath(prefix, object.Key)
			if err != nil {
				return err
			}
			if ok {
				err = d.Req.DeleteFile(object.Key)
				if err != nil {
					return parseError(path, d.Req.ParseError())
				}
			}
		}
		marker = list.NextMarker

		if len(list.Contents) == 0 || len(list.NextMarker) <= 0 {
			break
		}
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	methodString := "GET"
	method, ok := options["method"]
	if ok {
		methodString, ok = method.(string)
		if !ok || (methodString != "GET") {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	expiresIn := 20 * time.Minute 
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresIn = time.Until(et)
		}
	}

	return d.Req.GetPrivateURL(d.us3Path(path), expiresIn), nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func (d *driver) us3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

func (d *driver) getContent(key string, offset int64) ([]byte, error) {
	respBody, err := d.Req.DownloadFileRetRespBody(key, offset)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(respBody)
	respBody.Close()
	return data, err
}

func parseError(path string, err error) error {
	if us3Err, ok := err.(*ufsdk.Error); ok && us3Err.StatusCode == http.StatusNotFound && (us3Err.ErrMsg == "file not exist" || us3Err.RetCode == 0) {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return err
}

// US3 doesn't support dynamic blksize. So in some cases, we need some datas to stay in memory.
// For example, when our stationary blksize is 4MB and writer is going to upload object in multi
// continue stream appending parts like 6MB, 6MB and 6MB. The last 2MB in 1st and 3rd part will
// stay at memory to wait for next Write call.
// 
// 1st part            2nd part           3rd part
// [-------- | ----]  [---- | --------]  [-------- | ----]	6MB+6MB+6MB
// [++++++++] [++++    ++++] [++++++++]  [++++++++] [++++]	4MB+4MB+4MB+4MB+2MB
//             MEM                                   MEM
type tmpPart struct { // 暂存还未上传的 part
	ReadyPart   []byte
	PendingPart []byte
}

var TmpPartMap sync.Map // global map: <key, TmpPart>

// writer attempts to upload parts to US3 in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	state       *ufsdk.MultipartState
	parts       []*ufsdk.Part
	key         string // absolute path
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key string, state *ufsdk.MultipartState, parts []*ufsdk.Part) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += int64(part.Size)
	}
	var rdPart, pdPart []byte
	if part, ok := TmpPartMap.Load(key); ok {
		defer TmpPartMap.Delete(key)
		if tmpPart, ok := part.(tmpPart); ok {
			rdPart = tmpPart.ReadyPart
			pdPart = tmpPart.PendingPart
			size += int64(len(tmpPart.ReadyPart))
		}
	}
	return &writer{
		driver:      d,
		key:         key,
		state:       state,
		size:        size,
		parts:       parts,
		readyPart:   rdPart,
		pendingPart: pdPart,
	}
}

// Implement the storagedriver.FileWriter interface
func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	// If the last written part is smaller than minChunkSize, we need to make a
	// new multipart upload :sadface:
	if len(w.parts) > 0 && int((*w.parts[len(w.parts)-1]).Size) < w.state.BlkSize {
		err := w.driver.Req.FinishMultipartUpload(w.state)
		if err != nil {
			w.Cancel()
			return 0, err
		}

		state, err := w.driver.Req.InitiateMultipartUpload(w.key, w.driver.getContentType())
		if err != nil {
			return 0, err
		}
		w.state = state

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < int64(w.state.BlkSize) {
			contents, err := w.driver.getContent(w.key, 0)
			if err != nil {
				w.Cancel()
				return 0, parseError(w.key, w.driver.Req.ParseError())
			}
			w.parts = nil
			w.readyPart = contents
		} else {
			// Otherwise we can use the old file as the new first part
			
			// The api UploadPartCopy of US3 is still not supported. So we try to skip it and make sure
			// the program will never get to this point.
			w.driver.Req.AbortMultipartUpload(w.state)
			return 0, fmt.Errorf("UploadPartCopy is still not supported")
		}
	}

	var n int

	for len(p) > 0 {
		// If no parts are ready to write, fill up the first part
		if neededBytes := int(w.state.BlkSize) - len(w.readyPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.readyPart = append(w.readyPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
			} else {
				w.readyPart = append(w.readyPart, p...)
				n += len(p)
				// num += len(p)
				p = nil
			}
		}

		if neededBytes := int(w.state.BlkSize) - len(w.pendingPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.pendingPart = append(w.pendingPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
				err := w.flushPart()
				if err != nil {
					w.size += int64(n)
					w.Cancel()
					return n, err
				}
			} else {
				w.pendingPart = append(w.pendingPart, p...)
				n += len(p)
				p = nil
			}
		}
	}

	w.size += int64(n)

	return n, nil
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	err := w.flushPart()
	if err != nil {
		return err
	}
	TmpPartMap.Store(w.key, tmpPart{
		ReadyPart:   w.readyPart,
		PendingPart: w.pendingPart,
	})
	return nil
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	return w.driver.Req.AbortMultipartUpload(w.state)
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}
	w.committed = true
	for len(w.readyPart) > 0 {
		err := w.flushPart()
		if err != nil {
			return err
		}
	}
	err := w.driver.Req.FinishMultipartUpload(w.state)
	if err != nil {
		w.driver.Req.AbortMultipartUpload(w.state)
		return err
	}
	return nil
}

// flushPart flushes buffers to write a part to US3.
// Only called by Write (with both buffers full) and Close/Commit (always)
func (w *writer) flushPart() error {
	if len(w.readyPart) == 0 && len(w.pendingPart) == 0 {
		// nothing to write
		return nil
	}

	if !w.committed && len(w.readyPart) < w.state.BlkSize {
		return nil
	}

	part, err := w.driver.Req.UploadPartRetPart(bytes.NewBuffer(w.readyPart), w.state, len(w.parts)) // 将 readyPart 上传
	if err != nil {
		return err
	}

	w.parts = append(w.parts, part)
	w.readyPart = w.pendingPart
	w.pendingPart = nil
	return nil
}
