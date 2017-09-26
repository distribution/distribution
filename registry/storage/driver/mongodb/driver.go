package mongodb

import (
	"context"
	"errors"
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

const driverName = "mongodb"
const fs = "registry"
const separator = "/"

func init() {
	factory.Register(driverName, &mongodbDriverFactory{})
}

// mongodbDriverFactory implements the factory.StorageDriverFactory interface.
type mongodbDriverFactory struct{}

type driver struct {
	session *mgo.Session
	db      *mgo.Database
}

// baseEmbed allows us to hide the Base embed.
type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by a local map.
// Intended solely for example and testing purposes.
type Driver struct {
	baseEmbed // embedded, hidden base driver.
}

type gridFsEntry struct {
	ID       bson.ObjectId `bson:"_id,omitempty"`
	Filename string
	Length   int64
}

type databaseConfigEntry struct {
	ID          bson.ObjectId `bson:"_id,omitempty"`
	Partitioned bool
}

type collectionsConfigEntry struct {
	ID string `bson:"_id"`
}

type serverStatusEntry struct {
	Process string
}

type driverMode struct {
	consistencyMode int
	refresh         bool
}

var _ storagedriver.StorageDriver = &Driver{}

func (factory *mongodbDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// FromParameters constructs a new Driver with a given parameters map.
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	url, ok := parameters["url"]
	if !ok || fmt.Sprint(url) == "" {
		return nil, fmt.Errorf("No 'url' parameter provided")
	}
	databaseName, ok := parameters["dbname"]
	if !ok || fmt.Sprint(databaseName) == "" {
		databaseName = "docker"
	}

	mode, ok := parameters["dbmode"]
	if ok {
		dbMode := 2
		modeAsInt, err := strconv.Atoi(fmt.Sprint(mode))
		if err == nil {
			dbMode = modeAsInt
		}

		dbRefresh := false
		refresh, ok := parameters["dbrefresh"]
		if ok {
			refreshAsBool, err := strconv.ParseBool(fmt.Sprint(refresh))
			if err == nil {
				dbRefresh = refreshAsBool
			}
		}
		return New(fmt.Sprint(url), fmt.Sprint(databaseName), &driverMode{
			consistencyMode: dbMode,
			refresh:         dbRefresh,
		})
	}
	return New(fmt.Sprint(url), fmt.Sprint(databaseName), nil)
}

// New constructs a new Driver.
func New(url, databaseName string, mode *driverMode) (*Driver, error) {
	session, err := mgo.Dial(url)
	if err != nil {
		return nil, err
	}

	if mode != nil {
		session.SetMode(mgo.Mode(mode.consistencyMode), mode.refresh)
	}

	d := &driver{
		session: session,
		db:      session.DB(databaseName),
	}
	isShard, err := isShardEnvironment(d)
	if err != nil {
		return nil, err
	}
	if isShard {
		err := initShard(d)
		if err != nil {
			return nil, err
		}
	}
	return &Driver{baseEmbed: baseEmbed{Base: base.Base{StorageDriver: d}}}, nil
}

func isShardEnvironment(d *driver) (bool, error) {
	var serverStatus serverStatusEntry
	err := d.session.DB("admin").Run(bson.M{"serverStatus": 1}, &serverStatus)
	if err != nil {
		return false, err
	}
	return serverStatus.Process == "mongos", nil
}

func initShard(d *driver) error {
	err := createGridFSDatabase(d)
	if err != nil {
		return err
	}
	err = enableDatabaseSharding(d)
	if err != nil {
		return err
	}
	err = createShardKeys(d)
	if err != nil {
		return err
	}
	return nil
}

func createGridFSDatabase(d *driver) error {
	count, err := d.GridFS().Find(bson.M{}).Count()
	if err != nil {
		return err
	}
	if count == 0 {
		tempContent := make([]byte, 2)
		tempFilename := "temp_file"
		err := d.PutContent(nil, tempFilename, tempContent)
		if err != nil {
			return err
		}
		e := d.Delete(nil, tempFilename)
		if e != nil {
			return err
		}
	}
	return nil
}

func enableDatabaseSharding(d *driver) error {
	var configEntries []databaseConfigEntry
	findConfigError := d.session.DB("config").C("databases").Find(bson.M{"_id": d.db.Name}).All(&configEntries)
	if findConfigError != nil {
		return findConfigError
	}
	if len(configEntries) != 1 {
		return storagedriver.Error{
			DriverName: d.Name(),
			Enclosed:   errors.New("Cannot find database '" + d.db.Name + "' configuration"),
		}
	}
	if !configEntries[0].Partitioned {
		result := bson.M{}
		err := d.session.DB("admin").Run(bson.M{"enableSharding": d.db.Name}, &result)
		if err != nil {
			return err
		}
	}
	return nil
}

func createShardKeys(d *driver) error {
	chunksCollection := d.db.Name + "." + fs + ".chunks"
	err := createShardKey(d, chunksCollection, bson.D{
		{Name: "shardCollection", Value: chunksCollection},
		{Name: "key", Value: bson.D{
			{Name: "files_id", Value: 1},
			{Name: "n", Value: 1},
		}},
	})
	if err != nil {
		return err
	}

	filesCollection := d.db.Name + "." + fs + ".files"
	err = createShardKey(d, filesCollection, bson.D{
		{Name: "shardCollection", Value: filesCollection},
		{Name: "key", Value: bson.D{
			{Name: "_id", Value: 1},
		}},
	})
	if err != nil {
		return err
	}
	return nil
}

func createShardKey(d *driver, collectionName string, shardKeyCmd interface{}) error {
	result := bson.M{}
	var existingShardKeys []collectionsConfigEntry
	err := d.session.DB("config").C("collections").Find(bson.M{"_id": collectionName}).All(&existingShardKeys)
	if err != nil {
		return err
	}
	if len(existingShardKeys) == 0 {
		err := d.session.DB("admin").Run(shardKeyCmd, &result)
		if err != nil {
			return err
		}
	}
	return nil
}

// Implement the storagedriver.StorageDriver interface.

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	rc, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	result, err := ioutil.ReadAll(rc)
	closeErr := rc.Close()
	if closeErr != nil {
		return nil, closeErr
	}
	return result, err
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, p string, contents []byte) error {
	deleteErr := d.GridFS().Remove(p)
	if deleteErr != nil {
		return deleteErr
	}
	file, err := d.GridFS().Create(p)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(contents)
	if writeErr != nil {
		return writeErr
	}
	closeErr := file.Close()
	if closeErr != nil {
		return closeErr
	}
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	file, err := d.GridFS().Open(path)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}

	_, err = file.Seek(offset, 0)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	file, err := d.GridFS().Create(path)
	if err != nil {
		return nil, err
	}

	existingFile, err := d.GridFS().Open(path)
	if err == nil { //file exists
		defer existingFile.Close()
		if append {
			_, copyErr := io.Copy(file, existingFile)
			if copyErr != nil {
				return nil, copyErr
			}
		}
		err := d.GridFS().Remove(path)
		if err != nil {
			return nil, err
		}
	} else {
		if append {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
	}
	return d.newWriter(file), nil
}

// Stat returns info about the provided path.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	file, err := d.GridFS().Open(path)
	if err != nil {
		if err == mgo.ErrNotFound {
			return dirStat(d, path)
		}
		return nil, err
	}
	fi := storagedriver.FileInfoFields{
		Path:    path,
		IsDir:   false,
		ModTime: file.UploadDate(),
		Size:    file.Size(),
	}
	closeErr := file.Close()
	if closeErr != nil {
		return nil, closeErr
	}
	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

func dirStat(d *driver, path string) (storagedriver.FileInfo, error) {
	var files []gridFsEntry
	findErr := d.GridFS().Find(bson.M{"filename": bson.M{"$regex": bson.RegEx{Pattern: path + ".*"}}}).All(&files)
	if findErr != nil {
		return nil, findErr
	}
	if len(files) > 0 {
		return storagedriver.FileInfoInternal{FileInfoFields: storagedriver.FileInfoFields{
			Path:    path,
			IsDir:   true,
			ModTime: files[0].ID.Time(),
		}}, nil
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	var files []gridFsEntry
	if !strings.HasSuffix(path, separator) {
		path += separator
	}
	err := d.GridFS().Find(bson.M{"filename": bson.M{"$regex": bson.RegEx{Pattern: path + ".*"}}}).All(&files)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for i := 0; i < len(files); i++ {
		filename := files[i].Filename
		descendant := strings.TrimPrefix(filename, path)
		if descendant != filename {
			set[path+strings.SplitN(descendant, separator, 2)[0]] = true
		}
	}
	if path != separator && len(set) == 0 {
		return nil, storagedriver.PathNotFoundError{Path: strings.TrimSuffix(path, separator)}
	}

	result := make([]string, len(set))
	index := 0
	for key := range set {
		result[index] = key
		index++
	}
	return result, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	destFile, err := d.GridFS().Create(destPath)
	if err != nil {
		return err
	}
	sourceFile, err := d.Reader(ctx, sourcePath, 0)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	_, copyErr := io.Copy(destFile, sourceFile)
	if copyErr != nil {
		return copyErr
	}
	removeErr := d.GridFS().Remove(sourcePath)
	if removeErr != nil {
		return removeErr
	}
	return destFile.Close()
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	var files []gridFsEntry
	err := d.GridFS().Find(bson.M{"$or": []bson.M{
		{"filename": bson.M{"$regex": bson.RegEx{Pattern: path + "/.*"}}},
		{"filename": bson.M{"$eq": path}},
	}}).All(&files)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}
	for _, file := range files {
		err := d.GridFS().RemoveId(file.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod{}
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *driver) GridFS() *mgo.GridFS {
	err := d.session.Ping()
	if err != nil {
		logrus.Errorf("error while trying to reach mongodb, refreshing connection: %v", err)
		d.session.Refresh()
	}
	return d.db.GridFS(fs)
}

//**********************************************************************************************************************
// FileWriter implementation
//**********************************************************************************************************************
type writer struct {
	driver    *driver
	file      *mgo.GridFile
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(gridFile *mgo.GridFile) storagedriver.FileWriter {
	return &writer{
		driver: d,
		file:   gridFile,
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}
	return w.file.Write(p)
}

func (w *writer) Size() int64 {
	return w.file.Size()
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.file.Close()
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	w.file.Abort()

	return nil
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
	return nil
}
