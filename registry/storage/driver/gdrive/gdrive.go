// Package gdrive provides a storagedriver.StorageDriver implementation to
// store blobs in Google drive storage.
//
// This package leverages the google.golang.org/api/drive/v3 client library
// for interfacing with google drive.
//
// Parameters :
//
// keyFile : A private service account key file in JSON file format that can be downloaded
//           from google api console.
// rootdirectory : Folder name in google drive to store all registry files.
//
// +build include_gdrive

package gdrive

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	drive "google.golang.org/api/drive/v3"
)

const driverName = "gdrive"

// DriverParameters represents all configuration options available for the
// filesystem driver
type DriverParameters struct {
	KeyFile       string
	RootDirectory string
}

func init() {
	factory.Register(driverName, &gdriveDriverFactory{})
}

// gdriveDriverFactory implements the factory.StorageDriverFactory interface
type gdriveDriverFactory struct{}

func (factory *gdriveDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	rootID       string
	driveService *drive.Service
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Google Drive.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	params, err := fromParametersImpl(parameters)
	if err != nil || params == nil {
		return nil, err
	}
	return New(*params)
}

func fromParametersImpl(parameters map[string]interface{}) (*DriverParameters, error) {

	keyFile, ok := parameters["keyFile"]
	if !ok || fmt.Sprint(keyFile) == "" {
		return nil, fmt.Errorf("No keyFile parameter provided")
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok || fmt.Sprint(rootDirectory) == "" {
		return nil, fmt.Errorf("No rootDirectory parameter provided")
	}

	params := &DriverParameters{
		KeyFile:       fmt.Sprint(keyFile),
		RootDirectory: fmt.Sprint(rootDirectory),
	}
	return params, nil
}

// New constructs a new Driver with a given rootDirectory
func New(params DriverParameters) (*Driver, error) {
	//Read the config file
	jsonKey, err := ioutil.ReadFile(params.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("Unable to read client secret file: %v", err)
	}

	jwtConf := new(jwt.Config)

	jwtConf, err = google.JWTConfigFromJSON(jsonKey, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("JWTConfigFromJSON failed:%v", err)
	}

	// get the root directory id or create it if not present
	client := jwtConf.Client(oauth2.NoContext)
	driveService, err := drive.New(client)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize the google drive client.")
	}
	rootID := ""
	r, err := driveService.Files.List().Fields("files(id, name)").Do()
	if err != nil {
		return nil, fmt.Errorf("Cannot retrieve file details:%v", err)
	}
	if r != nil && len(r.Files) > 0 {
		for _, i := range r.Files {
			if i.Name == params.RootDirectory {
				rootID = i.Id
				break
			}
		}
	}
	if rootID == "" {
		dstFile := &drive.File{Name: params.RootDirectory,
			MimeType: "application/vnd.google-apps.folder"}
		res, err := driveService.Files.Create(dstFile).Do()
		if err != nil {
			return nil, fmt.Errorf("Failed to create directory: %s", err)
		}
		rootID = res.Id
	}

	gDriver := &driver{rootID: rootID, driveService: driveService}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: gDriver,
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
	rc, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	bytes, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, subPath string, contents []byte) error {
	writer, err := d.Writer(ctx, subPath, false)
	if err != nil {
		log.Fatalf("Unable to get the writer %v", err)
		return err
	}
	defer writer.Close()
	_, err = io.Copy(writer, bytes.NewReader(contents))
	if err != nil {
		writer.Cancel()
		return err
	}
	return writer.Commit()
}

// get the Google drive file object for a given filename
func (d *driver) getFile(path string) *drive.File {
	r, _ := d.driveService.Files.List().Q(`"` + d.rootID + `" in parents and name = "` + path + `"`).
		Fields("files(id, name, size, modifiedTime)").Do()
	if r != nil && len(r.Files) > 0 {
		return r.Files[0]
	}
	return nil
}

type bytesBuffer struct {
	*bytes.Buffer
}

func (b *bytesBuffer) Close() (err error) {
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	file := d.getFile(path)
	if file == nil {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	httpRes, _ := d.driveService.Files.Get(file.Id).Download()
	byteData, _ := ioutil.ReadAll(httpRes.Body)
	byteData = byteData[offset:]
	return &bytesBuffer{bytes.NewBuffer(byteData)}, nil
}

func (d *driver) Writer(ctx context.Context, subPath string, append bool) (storagedriver.FileWriter, error) {
	var size int
	var bytes []byte

	if append {
		file := d.getFile(subPath)
		if file != nil {
			httpRes, err := d.driveService.Files.Get(file.Id).Download()
			if err != nil {
				log.Fatalf("Unable to retrieve the file in drive: %s", err)
			}
			newText, _ := ioutil.ReadAll(httpRes.Body)
			size = len(newText)
			bytes = newText
		}

	}
	return newFileWriter(bytes, subPath, size, d), nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, subPath string) (storagedriver.FileInfo, error) {
	driveFileInfo := driveFileInfo{}
	file := d.getFile(subPath)
	if file != nil {
		driveFileInfo.modifiedTime = file.ModifiedTime
		driveFileInfo.size = file.Size
		driveFileInfo.isDir = false
		return fileInfo{
			path:          subPath,
			driveFileInfo: driveFileInfo,
		}, nil
	}

	r, _ := d.driveService.Files.List().Q(`"` + d.rootID + `" in parents`).Do()

	if r != nil && len(r.Files) > 0 {
		for _, i := range r.Files {
			if subPath != "/" && strings.HasPrefix(i.Name, subPath) {
				driveFileInfo.size = 0
				driveFileInfo.isDir = true
				return fileInfo{
					path:          subPath,
					driveFileInfo: driveFileInfo,
				}, nil
			}
		}
	}

	return nil, storagedriver.PathNotFoundError{Path: subPath}
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *driver) List(ctx context.Context, subPath string) ([]string, error) {
	r, _ := d.driveService.Files.List().Q(`"` + d.rootID + `" in parents`).Do()

	keys := []string{}
	checkMap := make(map[string]bool)
	if r != nil && len(r.Files) > 0 {
		for _, i := range r.Files {

			if subPath == "/" {
				index := strings.IndexByte(i.Name[1:], '/')
				if _, ok := checkMap[i.Name[:index+1]]; !ok {
					checkMap[i.Name[:index+1]] = true
					keys = append(keys, i.Name[:index+1])
				}
			} else {
				if strings.HasPrefix(i.Name, subPath) {
					index := strings.IndexByte(i.Name[len(subPath)+1:], '/')
					if index != -1 {
						index += len(subPath)
						if _, ok := checkMap[i.Name[:index+1]]; !ok {
							checkMap[i.Name[:index+1]] = true
							keys = append(keys, i.Name[:index+1])
						}
					} else {
						keys = append(keys, i.Name)
					}
				}
			}
		}
	}
	if subPath != "/" && len(keys) == 0 {
		return nil, storagedriver.PathNotFoundError{Path: subPath}
	}
	return keys, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	dstFile := &drive.File{}
	file := d.getFile(sourcePath)
	if file != nil {
		dstFile.Name = destPath
		_, err := d.driveService.Files.Update(file.Id, dstFile).Do()
		if err != nil {
			return err
		}
	} else {
		return storagedriver.PathNotFoundError{Path: sourcePath}
	}
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, subPath string) error {
	r, _ := d.driveService.Files.List().
		Q(`"` + d.rootID + `" in parents`).Do()
	pathFound := false
	if r != nil && len(r.Files) > 0 {
		for _, i := range r.Files {
			if subPath != "/" && strings.HasPrefix(i.Name, subPath) {
				pathFound = true
				err := d.driveService.Files.Delete(i.Id).Do()
				if err != nil {
					log.Fatalf("Cannot delete the file:%s %s", i.Name, err)
				}
			}
		}
	}
	if !pathFound {
		return storagedriver.PathNotFoundError{Path: subPath}
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod{}
}

type fileInfo struct {
	driveFileInfo driveFileInfo
	path          string
}

type driveFileInfo struct {
	modifiedTime string
	size         int64
	isDir        bool
}

// Path provides the full path of the target of this file info.
func (fi fileInfo) Path() string {
	return fi.path
}

// Size returns current length in bytes of the file. The return value can
// be used to write to the end of the file at path. The value is
// meaningless if IsDir returns true.
func (fi fileInfo) Size() int64 {
	return fi.driveFileInfo.size
}

// ModTime returns the modification time for the file. For backends that
// don't have a modification time, the creation time should be returned.
func (fi fileInfo) ModTime() time.Time {
	t, _ := time.Parse(time.RFC3339, fi.driveFileInfo.modifiedTime)
	return t
}

// IsDir returns true if the path is a directory.
func (fi fileInfo) IsDir() bool {
	return fi.driveFileInfo.isDir
}

type fileWriter struct {
	bytes     []byte
	path      string
	size      int
	driver    *driver
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(bytes []byte, path string, size int, d *driver) *fileWriter {
	return &fileWriter{
		bytes:  bytes,
		path:   path,
		size:   size,
		driver: d,
	}
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, fmt.Errorf("already closed")
	} else if fw.committed {
		return 0, fmt.Errorf("already committed")
	} else if fw.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	fw.bytes = append(fw.bytes, p...)
	fw.size = fw.size + len(p)
	return len(p), nil
}

func (fw *fileWriter) Size() int64 {
	return int64(fw.size)
}

func (fw *fileWriter) Close() error {

	if fw.closed || fw.committed {
		return nil
	}

	fw.closed = true
	file := fw.driver.getFile(fw.path)
	if file != nil {
		dstFile := &drive.File{}
		dstFile.Name = file.Name
		_, err := fw.driver.driveService.Files.Update(file.Id, dstFile).
			Media(strings.NewReader(string(fw.bytes))).Do()
		if err != nil {
			fmt.Println("Close:unable to update")
		}
		return nil
	}

	newFile := &drive.File{Name: fw.path}
	newFile.Parents = []string{fw.driver.rootID}
	_, err := fw.driver.driveService.Files.Create(newFile).
		Media(bytes.NewReader(fw.bytes)).Do()
	if err != nil {
		fmt.Println("Create file in drive failed", err)
	}
	return nil
}

func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}
	file := fw.driver.getFile(fw.path)
	err := fw.driver.driveService.Files.Delete(file.Id).Do()
	if err != nil {
		log.Fatalf("Cannot delete file in drive: %s %s ", fw.path, err)
	}
	fw.cancelled = true
	return nil
}

func (fw *fileWriter) Commit() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	} else if fw.committed {
		return fmt.Errorf("already committed")
	} else if fw.cancelled {
		return fmt.Errorf("already cancelled")
	}

	file := fw.driver.getFile(fw.path)
	if file != nil {
		return nil
	}

	newFile := &drive.File{Name: fw.path}
	newFile.Parents = []string{fw.driver.rootID}
	_, err := fw.driver.driveService.Files.Create(newFile).
		Media(bytes.NewReader(fw.bytes)).Do()
	if err != nil {
		fmt.Println("Commit:Create file in drive failed", err)
		return nil
	}
	fw.committed = true

	return nil
}
