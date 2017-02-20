package qingstor

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	logger "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"

	"github.com/yunify/qingstor-sdk-go/config"
	"github.com/yunify/qingstor-sdk-go/request"
	"github.com/yunify/qingstor-sdk-go/request/errors"
	"github.com/yunify/qingstor-sdk-go/service"
)

const driverName = "qs"

// 4MB
const minChunkSize = 4 << 20

// 1G
const maxChunkSize = 1 << 30

const defaultChunkSize = 2 * minChunkSize

const listMax = 1000

type qsDriverFactory struct{}

func (factory *qsDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

func init() {
	logger.Infof("init and register qs driver...")
	factory.Register(driverName, &qsDriverFactory{})
}

// DriverParameters A struct comtainer all parameters
type DriverParameters struct {
	AccessKey     string
	SecretKey     string
	Bucket        string
	Zone          string
	ChunkSize     int64
	UserAgent     string
	RootDirectory string
}

type driver struct {
	qsBucket      *service.Bucket
	Bucket        string
	ChunkSize     int64
	Zone          string
	RootDirectory string
}

type baseEmbed struct {
	base.Base
}

// Driver comments
type Driver struct {
	baseEmbed
}

// FromParameters init the Driver with parameters.
func FromParameters(parameters map[string]interface{}) (*Driver, error) {

	accessKey, ok := parameters["accesskey"]
	if !ok {
		return nil, fmt.Errorf("No accesskey provided")
	}
	secretKey, ok := parameters["secretkey"]
	if !ok {
		return nil, fmt.Errorf("No secretKey provided")
	}

	zone, ok := parameters["zone"]
	if !ok {
		return nil, fmt.Errorf("No zone provided")
	}

	bucket, ok := parameters["bucket"]
	if !ok {
		return nil, fmt.Errorf("No bucket provided")
	}

	chunkSize, err := getParameterAsInt64(parameters, "chunksize", defaultChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}

	userAgent := parameters["useragent"]
	if userAgent == nil {
		userAgent = ""
	}

	params := DriverParameters{
		AccessKey:     fmt.Sprint(accessKey),
		SecretKey:     fmt.Sprint(secretKey),
		Bucket:        fmt.Sprint(bucket),
		Zone:          fmt.Sprint(zone),
		ChunkSize:     chunkSize,
		UserAgent:     fmt.Sprint(userAgent),
		RootDirectory: fmt.Sprint(rootDirectory),
	}
	return New(params)
}

// New constructs a QingStor Driver with config.
func New(params DriverParameters) (*Driver, error) {
	qsConfig, err := config.New(params.AccessKey, params.SecretKey)
	if err != nil {
		return nil, err
	}
	qsService, err := service.Init(qsConfig)
	if err != nil {
		return nil, err
	}
	qsBucket, err := qsService.Bucket(params.Bucket, params.Zone)
	if err != nil {
		return nil, err
	}
	d := &driver{
		qsBucket:      qsBucket,
		Bucket:        params.Bucket,
		ChunkSize:     params.ChunkSize,
		Zone:          params.Zone,
		RootDirectory: params.RootDirectory,
	}
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

func getParameterAsInt64(parameters map[string]interface{}, name string, defaultt int64, min int64, max int64) (int64, error) {
	rv := defaultt
	param := parameters[name]
	switch v := param.(type) {
	case string:
		vv, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("%s parameter must be an integer, %v invalid", name, param)
		}
		rv = vv
	case int64:
		rv = v
	case int, uint, int32, uint32, uint64:
		rv = reflect.ValueOf(v).Convert(reflect.TypeOf(rv)).Int()
	case nil:
		// do nothing
	default:
		return 0, fmt.Errorf("invalid value for %s: %#v", name, param)
	}

	if rv < min || rv > max {
		return 0, fmt.Errorf("The %s %#v parameter should be a number between %d and %d (inclusive)", name, rv, min, max)
	}

	return rv, nil
}

// Name return the QingStor driver name.
func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	reader, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(reader)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	hashInBytes := md5.Sum(content)
	md5String := hex.EncodeToString(hashInBytes[:16])
	// Perform Put Object

	output, err := d.qsBucket.PutObject(d.qsPath(path), &service.PutObjectInput{
		ContentType: service.String(d.getContentType()),
		ContentMD5:  service.String(md5String),
		Body:        bytes.NewReader(content),
	})
	logger.Infof("Put content to path[%v] with md5[%v] size[%d] and err[%v] output[%+v] ", path, md5String, len(content), err, output)
	return parseError(path, err)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	resp, err := d.qsBucket.GetObject(d.qsPath(path), &service.GetObjectInput{
		Range: service.String(fmt.Sprint("bytes=" + strconv.FormatInt(offset, 10) + "-")),
	})
	logger.Infof("Read content from [%d] in key[%v] response[%v] err[%v]", offset, path, resp, err)
	if err != nil {
		if qsErr, ok := err.(*errors.QingStorError); ok && qsErr.Code == "invalid_range" {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}
		logger.Infof("Reader err[%v] resp[%v]", err, resp)
		return nil, parseError(path, err)
	}

	if service.IntValue(resp.StatusCode) != http.StatusPartialContent {
		if resp.Body != nil {
			resp.Body.Close()
		}
		return ioutil.NopCloser(bytes.NewReader(nil)), nil
	}
	return resp.Body, nil
}

func parseError(path string, err error) error {
	if qsErr, ok := err.(*errors.QingStorError); ok && qsErr.Code == "object_not_exists" {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return err
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	key := d.qsPath(path)
	resp, err := d.qsBucket.InitiateMultipartUpload(key, &service.InitiateMultipartUploadInput{
		ContentType: service.String(d.getContentType()),
	})

	if err != nil {
		return nil, err
	}

	uploadID := resp.UploadID
	if !append {
		d.qsBucket.AbortMultipartUpload(key, &service.AbortMultipartUploadInput{
			UploadID: uploadID,
		})
		newResp, newErr := d.qsBucket.InitiateMultipartUpload(key, &service.InitiateMultipartUploadInput{
			ContentType: service.String(d.getContentType()),
		})
		if newErr != nil {
			return nil, newErr
		}
		return d.newWriter(key, service.StringValue(newResp.UploadID), nil), nil
	}

	listResp, err := d.qsBucket.ListMultipart(key, &service.ListMultipartInput{
		UploadID: uploadID,
	})

	if err != nil {
		return nil, parseError(path, err)
	}

	if listResp.ObjectParts == nil {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return d.newWriter(key, service.StringValue(uploadID), listResp.ObjectParts), nil
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func (d *driver) newWriter(key, updateID string, parts []*service.ObjectPartType) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += int64(service.IntValue(part.Size))
	}
	return &writer{
		driver:   d,
		key:      key,
		uploadID: updateID,
		parts:    parts,
		size:     size,
	}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	key := d.qsPath(path)
	resp, err := d.qsBucket.ListObjects(&service.ListObjectsInput{
		Limit:  service.Int(1),
		Prefix: service.String(key),
	})

	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(resp.Keys) == 1 {
		if service.StringValue(resp.Keys[0].Key) != key {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = int64(service.IntValue(resp.Keys[0].Size))
			fi.ModTime = time.Unix(int64(service.IntValue(resp.Keys[0].Modified)), 0)
		}
	} else if len(resp.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && path[len(path)-1] != '/' {
		path = path + "/"
	}

	prefix := ""
	if d.qsPath("") == "" {
		prefix = "/"
	}

	key := d.qsPath(path)
	resp, err := d.qsBucket.ListObjects(&service.ListObjectsInput{
		Delimiter: service.String("/"),
		Limit:     service.Int(listMax),
		Prefix:    service.String(key),
	})

	if err != nil {
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range resp.Keys {
			files = append(files, strings.Replace(service.StringValue(key.Key), d.qsPath(""), prefix, 1))
		}
		for _, commonPrefix := range resp.CommonPrefixes {
			directories = append(directories, strings.Replace(service.StringValue(commonPrefix)[0:len(service.StringValue(commonPrefix))-1], d.qsPath(""), prefix, 1))
		}
		if service.StringValue(resp.NextMarker) != "" {
			respNext, err := d.qsBucket.ListObjects(&service.ListObjectsInput{
				Delimiter: service.String("/"),
				Limit:     service.Int(listMax),
				Prefix:    service.String(key),
				Marker:    resp.NextMarker,
			})
			resp = respNext
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	_, err := d.Stat(ctx, sourcePath)
	if err != nil {
		return parseError(sourcePath, err)
	}

	sKey := d.qsPath(sourcePath)
	tKey := d.qsPath(destPath)
	logger.Infof("Move key from [%v] to [%v]", sKey, tKey)

	output, err := d.qsBucket.PutObject(tKey, &service.PutObjectInput{
		XQSMoveSource: service.String("/" + d.Bucket + "/" + sKey),
	})

	if err != nil {
		logger.Warnf("Move key err[%v] reqID[%v]", err, output.RequestID)
		return parseError(sourcePath, err)
	}
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	objects := make([]*service.KeyType, 0, listMax)
	key := d.qsPath(path)
	listObjectsInput := &service.ListObjectsInput{
		Prefix: service.String(key),
		Limit:  service.Int(listMax),
	}
	logger.Infof("Delete keys in %v", key)
ListLoop:
	for {
		listOutput, err := d.qsBucket.ListObjects(listObjectsInput)
		if err != nil {
			logger.Errorf("List [%v],[%d] [%+v]", err, len(listOutput.Keys), listOutput.Keys)
			return storagedriver.PathNotFoundError{Path: path}
		}

		if len(listOutput.Keys) == 0 {
			break
		}

		for _, keyType := range listOutput.Keys {
			if len(*keyType.Key) > len(key) && (*keyType.Key)[len(key)] != '/' {
				break ListLoop
			}
			objects = append(objects, &service.KeyType{
				Key: keyType.Key,
			})
		}

		// requestData := map[string]interface{}{
		// 	"objects": objects,
		// 	"quiet":   true,
		// }
		// jsonBytes, err := json.Marshal(requestData)
		// if err != nil {
		// 	return err
		// }
		// md5Value := md5.Sum(jsonBytes)
		_, err = d.qsBucket.DeleteMultipleObjects(&service.DeleteMultipleObjectsInput{
			Objects: objects,
			Quiet:   service.Bool(true),
			//ContentMD5: base64.StdEncoding.EncodeToString(md5Value[:]),
		})

		if err != nil {
			return err
		}

		if service.StringValue(listOutput.NextMarker) == "" {
			break
		}

		listObjectsInput.Marker = listOutput.NextMarker
		objects = make([]*service.KeyType, 0, listMax)
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
		if !ok || (methodString != "GET" && methodString != "HEAD") {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	var req *request.Request
	var err error
	key := d.qsPath(path)
	switch methodString {
	case "GET":
		req, _, err = d.qsBucket.GetObjectRequest(key, &service.GetObjectInput{})
	case "HEAD":
		req, _, err = d.qsBucket.HeadObjectRequest(key, &service.HeadObjectInput{})
	default:
		panic("never")
	}

	if err != nil {
		return "", err
	}

	err = req.SignQuery(20 * 60)
	if err != nil {
		return "", err
	}
	logger.Infof("URL session[%v] method[%v]", req.HTTPRequest.URL.String(), methodString)
	return req.HTTPRequest.URL.String(), nil

}

func (d *driver) qsPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

type writer struct {
	driver      *driver
	uploadID    string
	key         string
	parts       []*service.ObjectPartType
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	if len(w.parts) > 0 && int(service.IntValue(w.parts[len(w.parts)-1].Size)) < minChunkSize {

		var completedUploadedParts []*service.ObjectPartType
		for _, part := range w.parts {
			completedUploadedParts = append(completedUploadedParts, &service.ObjectPartType{
				Etag:       part.Etag,
				PartNumber: part.PartNumber,
			})
		}
		_, err := w.driver.qsBucket.CompleteMultipartUpload(w.key, &service.CompleteMultipartUploadInput{
			UploadID:    service.String(w.uploadID),
			ObjectParts: completedUploadedParts,
		})
		if err != nil {
			logger.Infof("Last part is smaller than minChunkSize, AbortMultipartUploadInput")
			w.driver.qsBucket.AbortMultipartUpload(w.key, &service.AbortMultipartUploadInput{
				UploadID: service.String(w.uploadID),
			})
			return 0, err
		}

		resp, err := w.driver.qsBucket.InitiateMultipartUpload(w.key, &service.InitiateMultipartUploadInput{
			ContentType: service.String(w.driver.getContentType()),
		})

		if err != nil {
			return 0, err
		}
		w.uploadID = service.StringValue(resp.UploadID)

		if w.size < minChunkSize {
			resp, err := w.driver.qsBucket.GetObject(w.key, &service.GetObjectInput{})
			defer resp.Body.Close()
			if err != nil {
				logger.Warnf("Get object err[%v] resp[%v] ", err, resp)
				return 0, err
			}
			w.parts = nil
			w.readyPart, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return 0, err
			}
		} else {
			resp, err := w.driver.qsBucket.GetObject(w.key, &service.GetObjectInput{})
			defer resp.Body.Close()
			if err != nil {
				logger.Warnf("Get object err[%v]  resp[%v]", err, resp)
				return 0, err
			}
			w.parts = nil
			w.readyPart, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return 0, err
			}
		}

	}

	var n int
	for len(p) > 0 {
		if neededBytes := int(w.driver.ChunkSize) - len(w.readyPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.readyPart = append(w.readyPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
			} else {
				w.readyPart = append(w.readyPart, p...)
				n += len(p)
				p = nil
			}
		}

		if neededBytes := int(w.driver.ChunkSize) - len(w.pendingPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.pendingPart = append(w.pendingPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
				err := w.flushPart()
				if err != nil {
					w.size += int64(n)
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

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.flushPart()
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true

	_, err := w.driver.qsBucket.AbortMultipartUpload(w.key, &service.AbortMultipartUploadInput{
		UploadID: service.String(w.uploadID),
	})
	logger.Infof("Cancel multipartupload reqID[%v] err[%v]", w.uploadID, err)
	return err
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}

	err := w.flushPart()
	if err != nil {
		return err
	}
	w.committed = true
	var totalSize int
	var completedUploadedParts []*service.ObjectPartType
	for _, part := range w.parts {
		totalSize += service.IntValue(part.Size)
		completedUploadedParts = append(completedUploadedParts, &service.ObjectPartType{
			PartNumber: part.PartNumber,
		})
	}

	resp, err := w.driver.qsBucket.CompleteMultipartUpload(w.key, &service.CompleteMultipartUploadInput{
		UploadID:    service.String(w.uploadID),
		ObjectParts: completedUploadedParts,
	})
	logger.Infof("Commit writer totalSize [%v] err [%v] key [%v] reqID [%v]", totalSize, err, w.key, w.uploadID)
	if err != nil {
		logger.Warnf("Complete multipart err[%v]  resp[%v]", err, resp)
		w.driver.qsBucket.AbortMultipartUpload(w.key, &service.AbortMultipartUploadInput{
			UploadID: service.String(w.uploadID),
		})
		return err
	}
	return nil
}

func (w *writer) flushPart() error {
	if len(w.readyPart) == 0 && len(w.pendingPart) == 0 {
		return nil
	}
	if len(w.pendingPart) < int(w.driver.ChunkSize) {
		w.readyPart = append(w.readyPart, w.pendingPart...)
		w.pendingPart = nil
	}

	partNumber := len(w.parts) + 1
	hashInBytes := md5.Sum(w.readyPart)
	md5String := hex.EncodeToString(hashInBytes[:16])

	output, err := w.driver.qsBucket.UploadMultipart(w.key, &service.UploadMultipartInput{
		PartNumber:    service.Int(partNumber),
		UploadID:      service.String(w.uploadID),
		ContentLength: service.Int(len(w.readyPart)),
		ContentMD5:    service.String(md5String),
		Body:          bytes.NewReader(w.readyPart),
	})
	logger.Infof("Flush part size[%d] md5[%v] req[%v]", len(w.readyPart), md5String, output)

	if err != nil {
		return err
	}

	w.parts = append(w.parts, &service.ObjectPartType{
		Created:    service.Time(time.Now()),
		Etag:       service.String(md5String),
		PartNumber: service.Int(partNumber),
		Size:       service.Int(len(w.readyPart)),
	})
	w.readyPart = w.pendingPart
	w.pendingPart = nil
	return nil
}
