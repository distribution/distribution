package speedy

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const driverName = "speedy"

type DriverParameters struct {
	storageUrls       string
	chunkSize         uint64
	heartBeatInterval int
}

type driver struct {
	storageUrlArr     []string
	healthUrlArr      []string
	chunkSize         uint64
	heartBeatInterval int
	client            *SpeedyClient
	rwlock            sync.RWMutex
}

type basedEmbed struct {
	base.Base
}

type Driver struct {
	basedEmbed
}

//ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
//given byte offset.
type readStreamReader struct {
	driver       *driver
	path         string
	index        int
	infoArr      []*MetaInfoValue
	remain       []byte
	remainOffset uint64 //read from remain buffer at index of remainOffset
	size         uint64 //total size
	offset       uint64 //global offset
}

func init() {
	factory.Register(driverName, &speedyDriverFactory{})
}

type speedyDriverFactory struct{}

func (factory *speedyDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// FromParameters constructs a new Driver with a given paramters map.
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	storageUrl, ok := parameters["storageurl"]
	if !ok {
		return nil, fmt.Errorf("No storageurl parameter provided")
	}

	chunkSizeParam, ok := parameters["chunksize"]
	if !ok {
		return nil, fmt.Errorf("No chunksize parameter provided")
	}

	chunkSizeMB, ok := chunkSizeParam.(int)
	if !ok {
		return nil, fmt.Errorf("The chunksize parameter should be a number")
	}

	//change MB to B
	chunkSize := chunkSizeMB << 20

	heartBeatIntervalParam, ok := parameters["heartbeatinterval"]
	if !ok {
		return nil, fmt.Errorf("No heartbeatinterval parameter provided")
	}

	heartBeatInterval, ok := heartBeatIntervalParam.(int)
	if !ok {
		return nil, fmt.Errorf("The heartbeatinterval parameter should be a number")
	}

	params := DriverParameters{
		storageUrls:       fmt.Sprint(storageUrl),
		chunkSize:         uint64(chunkSize),
		heartBeatInterval: heartBeatInterval,
	}

	return New(params)
}

//New constructs a new Driver with the speedy param.
func New(params DriverParameters) (*Driver, error) {
	storageUrlArr := strings.Split(params.storageUrls, ";")
	if len(storageUrlArr) < 1 {
		return nil, fmt.Errorf("The storageUrl parameter may be error")
	}

	client := &SpeedyClient{}

	d := &driver{
		storageUrlArr:     storageUrlArr,
		healthUrlArr:      make([]string, 0),
		chunkSize:         params.chunkSize,
		heartBeatInterval: params.heartBeatInterval,
		client:            client,
	}

	finalDriver := &Driver{
		basedEmbed: basedEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}

	go d.healthCheck(d.heartBeatInterval)
	return finalDriver, nil
}

func (d *driver) Name() string {
	return driverName
}

func (d *driver) updateHealthUrlArr() {
	healthUrlArr := make([]string, 0)
	for i, _ := range d.storageUrlArr {
		err := d.client.Ping(d.storageUrlArr[i])
		if err != nil {
			log.Errorf("speedy driver ping error: %v", err)
			continue
		}
		healthUrlArr = append(healthUrlArr, d.storageUrlArr[i])
	}
	d.rwlock.Lock()
	d.healthUrlArr = healthUrlArr
	d.rwlock.Unlock()
}

func (d *driver) healthCheck(seconds int) {
	timer := time.NewTicker(time.Duration(seconds) * time.Second)
	for {
		select {
		case <-timer.C:
			d.updateHealthUrlArr()
		}
	}
}

//GetContent retrives the content stored at the "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	url, err := d.getUrl()
	if err != nil {
		return nil, err
	}

	infoArr, err := d.client.GetFileInfo(url, path)
	if err != nil {
		return nil, err
	}

	if infoArr == nil && err == nil {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	if len(infoArr) == 0 {
		return nil, fmt.Errorf("There is not file info, path: %v", path)
	}

	if len(infoArr) != 1 {
		return nil, fmt.Errorf("metainfo maybe error, path: %v, metainfo: %v", path, infoArr)
	}

	data, err := d.client.DownloadFile(url, path, infoArr[0])
	if err != nil {
		return nil, err
	}

	return data, nil
}

//PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	url, err := d.getUrl()
	if err != nil {
		return err
	}

	info := &MetaInfoValue{
		Index:  0,
		Start:  0,
		End:    uint64(len(contents)),
		IsLast: true,
	}

	err = d.client.UploadFile(url, path, info, contents)
	if err != nil {
		return err
	}

	return nil
}

func (r *readStreamReader) Read(b []byte) (n int, err error) {
	bufferOffset := uint64(0)
	bufferSize := uint64(len(b))

	if bufferSize > r.size-r.offset {
		bufferSize = r.size - r.offset
	}

	// Fill b
	for bufferOffset < bufferSize {
		unreadRemainSize := uint64(len(r.remain)) - r.remainOffset
		fillSize := bufferSize - bufferOffset

		if unreadRemainSize >= fillSize {
			copy(b[bufferOffset:bufferSize], r.remain[r.remainOffset:r.remainOffset+fillSize])
			r.remainOffset += fillSize
			r.offset += fillSize
			return int(bufferSize), nil
		}

		if unreadRemainSize > 0 && unreadRemainSize <= fillSize {
			copy(b[bufferOffset:], r.remain[r.remainOffset:len(r.remain)])
			readSize := uint64(len(r.remain)) - r.remainOffset
			bufferOffset += readSize
			r.remainOffset = uint64(len(r.remain))
			r.offset += readSize
		}

		url, err := r.driver.getUrl()
		if err != nil {
			return int(bufferOffset), err
		}

		r.remain, err = r.driver.client.DownloadFile(url, r.path, r.infoArr[r.index])
		if err != nil {
			return int(bufferOffset), err
		}
		r.remainOffset = 0
		r.index++
	}

	return 0, fmt.Errorf("The capacity of buffer is empty")
}

func (r *readStreamReader) Close() error {
	r.remain = nil
	r.index = 0
	r.remainOffset = 0
	r.infoArr = nil
	return nil
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (d *driver) ReadStream(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	//get all metainfos
	url, err := d.getUrl()
	if err != nil {
		return nil, err
	}

	infoArr, err := d.client.GetFileInfo(url, path)
	if err != nil {
		return nil, err
	}

	if len(infoArr) == 0 {
		return nil, fmt.Errorf("There is not file info, path: %v", path)
	}

	return d.initReadStream(path, uint64(offset), infoArr)
}

func (d *driver) initReadStream(path string, offset uint64, infoArr []*MetaInfoValue) (io.ReadCloser, error) {
	var size uint64
	for index, _ := range infoArr {
		size += infoArr[index].End - infoArr[index].Start
	}

	if uint64(offset) > size {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: int64(offset)}
	}

	readCloser := &readStreamReader{
		driver:       d,
		path:         path,
		index:        0,
		infoArr:      infoArr,
		remain:       nil,
		remainOffset: 0,
		size:         size,
		offset:       0,
	}

	index := 0
	var readOffset uint64 = 0
	for index < len(infoArr) {
		nextReadOffset := readOffset + (infoArr[index].End - infoArr[index].Start)
		if nextReadOffset >= uint64(offset) {
			break
		}
		readOffset = nextReadOffset
		index++
		continue
	}

	url, err := d.getUrl()
	if err != nil {
		return nil, err
	}

	readCloser.remain, err = d.client.DownloadFile(url, path, infoArr[index])
	if err != nil {
		return nil, err
	}
	readCloser.index = index + 1
	readCloser.remainOffset = offset - readOffset
	readCloser.offset = offset
	return readCloser, nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a
// location designated by the given path.
// May be used to resume writing a stream by providing a nonzero offset.
// The offset must be no larger than the CurrentSize for this path.
func (d *driver) WriteStream(ctx context.Context, path string, offset int64, reader io.Reader) (int64, error) {
	url, err := d.getUrl()
	if err != nil {
		return 0, err
	}

	infoArr, err := d.client.GetFileInfo(url, path)
	if err != nil {
		return 0, err
	}

	var totalRead int64
	totalRead = 0
	index := 0
	var currentSize uint64 = 0

	//Align to chunk size, skip already exist chunk
	if len(infoArr) != 0 {
		for _, info := range infoArr {
			currentSize += info.End - info.Start
			index++
		}

		if currentSize < uint64(offset) {
			return 0, fmt.Errorf("speedy driver currentSize: %d < offset: %d", currentSize, offset)
		}

		//read 1MB every time
		var tempSize uint64 = 1 << 20
		tempBuf := make([]byte, tempSize)
		var distance uint64 = 0
		if currentSize > uint64(offset) {
			distance = currentSize - uint64(offset)
		}

		for distance > 0 {
			var readSize uint64 = 0
			if distance > tempSize {
				readSize = tempSize
			} else {
				readSize = distance
			}

			n, err := reader.Read(tempBuf[0:readSize])
			if err != nil {
				return totalRead, err
			}

			totalRead += int64(n)
			offset += int64(n)

			distance = currentSize - uint64(offset)
		}
	}

	readSize, err := d.writeStreamToSpeedy(path, currentSize, reader, index)
	totalRead += readSize
	if err != nil {
		return totalRead, err
	}

	return totalRead, nil
}

func (d *driver) writeStreamToSpeedy(path string, currentOffset uint64, reader io.Reader, index int) (totalRead int64, err error) {
	totalRead = 0
	buf := make([]byte, d.chunkSize)
	isLast := false

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Errorf("speedy driver writeStreamToSpeedy error: %v", err)
				return totalRead, err
			}
			isLast = true
		}

		info := &MetaInfoValue{
			Index:  uint64(index),
			Start:  currentOffset,
			End:    currentOffset + uint64(n),
			IsLast: isLast,
		}

		url, err := d.getUrl()
		if err != nil {
			return totalRead, err
		}

		err = d.client.UploadFile(url, path, info, buf[0:n])
		if err != nil {
			return totalRead, err
		}

		currentOffset = currentOffset + uint64(n)
		totalRead += int64(n)
		index++
		if isLast {
			return totalRead, nil
		}
	}
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	url, err := d.getUrl()
	if err != nil {
		return nil, err
	}

	infoArr, err := d.client.GetFileInfo(url, path)
	if err != nil {
		return nil, err
	}

	if err == nil && infoArr == nil {
		descendants, err := d.client.GetDirectDescendantPath(url, path)
		if err == nil && len(descendants) != 0 {
			return storagedriver.FileInfoInternal{
				FileInfoFields: storagedriver.FileInfoFields{
					Path:  path,
					Size:  0,
					IsDir: true,
				},
			}, nil
		}
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	if len(infoArr) == 0 {
		return nil, fmt.Errorf("There is not file info, path: %v", path)
	}

	var totalSize uint64
	modTime := infoArr[0].ModTime
	for index, _ := range infoArr {
		totalSize += infoArr[index].End - infoArr[index].Start
		if infoArr[index].ModTime.After(modTime) {
			modTime = infoArr[index].ModTime
		}
	}

	return storagedriver.FileInfoInternal{
		FileInfoFields: storagedriver.FileInfoFields{
			Path:    path,
			Size:    int64(totalSize),
			ModTime: modTime,
		},
	}, nil
}

// List returns a list of the objects that are direct descendants of the
//given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	url, err := d.getUrl()
	if err != nil {
		return nil, err
	}

	descendants, err := d.client.GetDirectDescendantPath(url, path)
	if err != nil {
		log.Errorf("speedy driver List error: %v", err)
		return nil, err
	}

	if err == nil && descendants == nil {
		return make([]string, 0), nil
	}

	return descendants, nil
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
// Note: This may be no more efficient than a copy followed by a delete for
// many implementations.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	url, err := d.getUrl()
	if err != nil {
		return err
	}

	return d.client.MoveFile(url, sourcePath, destPath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	url, err := d.getUrl()
	if err != nil {
		return err
	}

	return d.client.DeleteFile(url, path)
}

// URLFor returns a URL which may be used to retrieve the content stored at
// the given path, possibly using the given options.
// May return an ErrUnsupportedMethod in certain StorageDriver
// implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod
}

func (d *driver) getUrl() (string, error) {
	d.rwlock.RLock()
	healthUrlArr := d.healthUrlArr
	d.rwlock.RUnlock()
	//try to get url from healthUrlArr
	healthSize := len(healthUrlArr)
	if healthSize != 0 {
		index := rand.Int() % healthSize
		url := healthUrlArr[index]
		return url, nil
	}

	//try to get url from storageUrlArr
	totalSize := len(d.storageUrlArr)
	if totalSize == 0 {
		return "", fmt.Errorf("The storageUrlArr is empty")
	}
	index := rand.Int() % totalSize
	url := d.storageUrlArr[index]
	return url, nil
}
