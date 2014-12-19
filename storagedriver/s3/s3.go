package s3

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crowdmob/goamz/aws"
	"github.com/crowdmob/goamz/s3"
	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/factory"
)

const driverName = "s3"

// minChunkSize defines the minimum multipart upload chunk size
// S3 API requires multipart upload chunks to be at least 5MB
const chunkSize = 5 * 1024 * 1024

// listMax is the largest amount of objects you can request from S3 in a list call
const listMax = 1000

func init() {
	factory.Register(driverName, &s3DriverFactory{})
}

// s3DriverFactory implements the factory.StorageDriverFactory interface
type s3DriverFactory struct{}

func (factory *s3DriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// Driver is a storagedriver.StorageDriver implementation backed by Amazon S3
// Objects are stored at absolute keys in the provided bucket
type Driver struct {
	S3            *s3.S3
	Bucket        *s3.Bucket
	Encrypt       bool
	rootDirectory string
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - accesskey
// - secretkey
// - region
// - bucket
// - encrypt
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	accessKey, _ := parameters["accesskey"]
	secretKey, _ := parameters["secretkey"]

	regionName, ok := parameters["region"]
	if !ok || fmt.Sprint(regionName) == "" {
		return nil, fmt.Errorf("No region parameter provided")
	}
	region := aws.GetRegion(fmt.Sprint(regionName))
	if region.Name == "" {
		return nil, fmt.Errorf("Invalid region provided: %v", region)
	}

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	encrypt, ok := parameters["encrypt"]
	if !ok {
		return nil, fmt.Errorf("No encrypt parameter provided")
	}

	encryptBool, err := strconv.ParseBool(fmt.Sprint(encrypt))
	if err != nil {
		return nil, fmt.Errorf("Unable to parse the encrypt parameter: %v", err)
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		return nil, fmt.Errorf("No rootDirectory parameter provided")
	}

	return New(fmt.Sprint(accessKey), fmt.Sprint(secretKey), fmt.Sprint(bucket), fmt.Sprint(rootDirectory), region, encryptBool)
}

// New constructs a new Driver with the given AWS credentials, region, encryption flag, and
// bucketName
func New(accessKey, secretKey, bucketName, rootDirectory string, region aws.Region, encrypt bool) (*Driver, error) {
	auth, err := aws.GetAuth(accessKey, secretKey, "", time.Time{})
	if err != nil {
		return nil, err
	}

	s3obj := s3.New(auth, region)
	bucket := s3obj.Bucket(bucketName)

	if err := bucket.PutBucket(getPermissions()); err != nil {
		s3Err, ok := err.(*s3.Error)
		if !(ok && s3Err.Code == "BucketAlreadyOwnedByYou") {
			return nil, err
		}
	}

	// TODO What if they use this bucket for other things? I can't just clean out the multis
	// TODO Add timestamp checking
	multis, _, err := bucket.ListMulti("", "")
	if err != nil {
		return nil, err
	}

	for _, multi := range multis {
		err := multi.Abort()
		//TODO appropriate to do this error checking?
		if err != nil {
			return nil, err
		}
	}

	return &Driver{s3obj, bucket, encrypt, rootDirectory}, nil
}

// Implement the storagedriver.StorageDriver interface

// GetContent retrieves the content stored at "path" as a []byte.
func (d *Driver) GetContent(path string) ([]byte, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	content, err := d.Bucket.Get(d.s3Path(path))
	if err != nil {
		return nil, parseError(path, err)
	}
	return content, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *Driver) PutContent(path string, contents []byte) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	return parseError(path, d.Bucket.Put(d.s3Path(path), contents, d.getContentType(), getPermissions(), d.getOptions()))
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *Driver) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	if offset < 0 {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	headers := make(http.Header)
	headers.Add("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")

	resp, err := d.Bucket.GetResponseWithHeaders(d.s3Path(path), headers)
	if err != nil {
		if s3Err, ok := err.(*s3.Error); ok && s3Err.Code == "InvalidRange" {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}
	return resp.Body, nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a location
// designated by the given path.
func (d *Driver) WriteStream(path string, offset int64, reader io.Reader) (totalRead int64, err error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return 0, storagedriver.InvalidPathError{Path: path}
	}

	if offset < 0 {
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	partNumber := 1
	bytesRead := 0
	parts := []s3.Part{}
	var part s3.Part

	multi, err := d.Bucket.InitMulti(d.s3Path(path), d.getContentType(), getPermissions(), d.getOptions())
	if err != nil {
		return 0, err
	}

	buf := make([]byte, chunkSize)
	zeroBuf := make([]byte, chunkSize)

	// We never want to leave a dangling multipart upload, our only consistent state is
	// when there is a whole object at path. This is in order to remain consistent with
	// the stat call.
	//
	// Note that if the machine dies before executing the defer, we will be left with a dangling
	// multipart upload, which will eventually be cleaned up, but we will lose all of the progress
	// made prior to the machine crashing.
	defer func() {
		if len(parts) > 0 {
			if multi == nil {
				// Parts should be empty if the multi is not initialized
				panic("Unreachable")
			} else {
				if multi.Complete(parts) != nil {
					multi.Abort()
				}
			}
		}
	}()

	// Fills from 0 to total from current
	fromSmallCurrent := func(total int64) error {
		current, err := d.ReadStream(path, 0)
		if err != nil {
			return err
		}

		bytesRead = 0
		for int64(bytesRead) < total {
			//The loop should very rarely enter a second iteration
			nn, err := io.ReadFull(current, buf[bytesRead:total])
			bytesRead += nn
			if err != nil {
				// If you can't read the total contents, this means you lack access to the current file
				return err
			}

		}
		return nil
	}

	// Fills from parameter to chunkSize from reader
	fromReader := func(from int64) error {
		bytesRead = 0
		for int64(bytesRead) < chunkSize {
			nn, err := io.ReadFull(reader, buf[from+int64(bytesRead):])
			totalRead += int64(nn)
			bytesRead += nn

			if err != nil {
				if err != io.ErrUnexpectedEOF && err != io.EOF {
					return err
				}

				break
			}
		}

		part, err = multi.PutPart(int(partNumber), bytes.NewReader(buf[0:int64(bytesRead)+from]))
		if err != nil {
			return err
		}

		return nil
	}

	if offset > 0 {
		resp, err := d.Bucket.Head(d.s3Path(path), nil)
		if err != nil {
			if s3Err, ok := err.(*s3.Error); !ok || s3Err.Code != "NoSuchKey" {
				return 0, err
			}
		}

		currentLength := int64(0)
		if err == nil {
			currentLength = resp.ContentLength
		}

		if currentLength >= offset {
			if offset < chunkSize {
				// chunkSize > currentLength >= offset
				if err = fromSmallCurrent(offset); err != nil {
					return totalRead, err
				}

				if err = fromReader(offset); err != nil {
					return totalRead, err
				}
			} else {
				// currentLength >= offset >= chunkSize
				_, part, err = multi.PutPartCopy(partNumber,
					s3.CopyOptions{CopySourceOptions: "bytes=0-" + strconv.FormatInt(offset-1, 10)},
					d.Bucket.Name+"/"+d.s3Path(path))
				if err != nil {
					return 0, err
				}
			}

			parts = append(parts, part)
			partNumber++

			if totalRead+offset < chunkSize {
				return totalRead, nil
			}
		} else {
			// Fills between parameters with 0s but only when to - from <= chunkSize
			fromZeroFillSmall := func(from, to int64) error {
				bytesRead = 0
				for from+int64(bytesRead) < to {
					nn, err := io.ReadFull(bytes.NewReader(zeroBuf), buf[from+int64(bytesRead):to])
					bytesRead += nn
					if err != nil {
						return err
					}
				}

				return nil
			}

			// Fills between parameters with 0s, making new parts
			fromZeroFillLarge := func(from, to int64) error {
				bytesRead64 := int64(0)
				for to-(from+bytesRead64) >= chunkSize {
					part, err := multi.PutPart(int(partNumber), bytes.NewReader(zeroBuf))
					if err != nil {
						return err
					}
					bytesRead64 += chunkSize

					parts = append(parts, part)
					partNumber++
				}

				bytesRead = 0
				for from+bytesRead64+int64(bytesRead) < to {
					nn, err := io.ReadFull(bytes.NewReader(zeroBuf), buf[0+bytesRead:(to-from)%chunkSize])
					bytesRead64 += int64(nn)
					if err != nil {
						return err
					}
				}

				return nil
			}

			// currentLength < offset
			if currentLength < chunkSize {
				if offset < chunkSize {
					// chunkSize > offset > currentLength
					if err = fromSmallCurrent(currentLength); err != nil {
						return totalRead, err
					}

					if err = fromZeroFillSmall(currentLength, offset); err != nil {
						return totalRead, err
					}

					if err = fromReader(offset); err != nil {
						return totalRead, err
					}

					part, err = multi.PutPart(int(partNumber), bytes.NewReader(buf))
					if err != nil {
						return totalRead, err
					}

					parts = append(parts, part)
					partNumber++

					if totalRead+offset < chunkSize {
						return totalRead, nil
					}
				} else {
					// offset >= chunkSize > currentLength
					if err = fromSmallCurrent(currentLength); err != nil {
						return totalRead, err
					}

					if err = fromZeroFillSmall(currentLength, chunkSize); err != nil {
						return totalRead, err
					}

					part, err = multi.PutPart(int(partNumber), bytes.NewReader(buf))
					if err != nil {
						return totalRead, err
					}

					parts = append(parts, part)
					partNumber++

					//Zero fill from chunkSize up to offset, then some reader
					if err = fromZeroFillLarge(chunkSize, offset); err != nil {
						return totalRead, err
					}

					if err = fromReader(offset % chunkSize); err != nil {
						return totalRead, err
					}

					part, err = multi.PutPart(int(partNumber), bytes.NewReader(buf))
					if err != nil {
						return totalRead, err
					}

					parts = append(parts, part)
					partNumber++

					if totalRead+(offset%chunkSize) < chunkSize {
						return totalRead, nil
					}
				}
			} else {
				// offset > currentLength >= chunkSize
				_, part, err = multi.PutPartCopy(partNumber,
					s3.CopyOptions{CopySourceOptions: "bytes=0-" + strconv.FormatInt(currentLength-1, 10)},
					d.Bucket.Name+"/"+d.s3Path(path))
				if err != nil {
					return 0, err
				}

				parts = append(parts, part)
				partNumber++

				//Zero fill from currentLength up to offset, then some reader
				if err = fromZeroFillLarge(currentLength, offset); err != nil {
					return totalRead, err
				}

				if err = fromReader((offset - currentLength) % chunkSize); err != nil {
					return totalRead, err
				}

				part, err = multi.PutPart(int(partNumber), bytes.NewReader(buf))
				if err != nil {
					return totalRead, err
				}

				parts = append(parts, part)
				partNumber++

				if totalRead+((offset-currentLength)%chunkSize) < chunkSize {
					return totalRead, nil
				}
			}

		}
	}

	for {
		bytesRead, err := io.ReadFull(reader, buf)
		totalRead += int64(bytesRead)

		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return totalRead, err
		}

		part, err := multi.PutPart(int(partNumber), bytes.NewReader(buf[0:bytesRead]))
		if err != nil {
			return totalRead, err
		}

		parts = append(parts, part)
		partNumber++

		if int64(bytesRead) < chunkSize {
			break
		}
	}

	return totalRead, nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *Driver) Stat(path string) (storagedriver.FileInfo, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	listResponse, err := d.Bucket.List(d.s3Path(path), "", "", 1)
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(listResponse.Contents) == 1 {
		if listResponse.Contents[0].Key != d.s3Path(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = listResponse.Contents[0].Size

			timestamp, err := time.Parse(time.RFC3339Nano, listResponse.Contents[0].LastModified)
			if err != nil {
				return nil, err
			}
			fi.ModTime = timestamp
		}
	} else if len(listResponse.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *Driver) List(path string) ([]string, error) {
	if !storagedriver.PathRegexp.MatchString(path) && path != "/" {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	if path != "/" && path[len(path)-1] != '/' {
		path = path + "/"
	}
	listResponse, err := d.Bucket.List(d.s3Path(path), "/", "", listMax)
	if err != nil {
		return nil, err
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range listResponse.Contents {
			files = append(files, strings.Replace(key.Key, d.s3Path(""), "", 1))
		}

		for _, commonPrefix := range listResponse.CommonPrefixes {
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.s3Path(""), "", 1))
		}

		if listResponse.IsTruncated {
			listResponse, err = d.Bucket.List(d.s3Path(path), "/", listResponse.NextMarker, listMax)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *Driver) Move(sourcePath string, destPath string) error {
	if !storagedriver.PathRegexp.MatchString(sourcePath) {
		return storagedriver.InvalidPathError{Path: sourcePath}
	} else if !storagedriver.PathRegexp.MatchString(destPath) {
		return storagedriver.InvalidPathError{Path: destPath}
	}

	/* This is terrible, but aws doesn't have an actual move. */
	_, err := d.Bucket.PutCopy(d.s3Path(destPath), getPermissions(),
		s3.CopyOptions{Options: d.getOptions(), ContentType: d.getContentType()}, d.Bucket.Name+"/"+d.s3Path(sourcePath))
	if err != nil {
		return parseError(sourcePath, err)
	}

	return d.Delete(sourcePath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *Driver) Delete(path string) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	listResponse, err := d.Bucket.List(d.s3Path(path), "", "", listMax)
	if err != nil || len(listResponse.Contents) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	s3Objects := make([]s3.Object, listMax)

	for len(listResponse.Contents) > 0 {
		for index, key := range listResponse.Contents {
			s3Objects[index].Key = key.Key
		}

		err := d.Bucket.DelMulti(s3.Delete{Quiet: false, Objects: s3Objects[0:len(listResponse.Contents)]})
		if err != nil {
			return nil
		}

		listResponse, err = d.Bucket.List(d.s3Path(path), "", "", listMax)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) s3Path(path string) string {
	return strings.TrimLeft(d.rootDirectory+path, "/")
}

func (d *Driver) fullPath(path string) string {
	return d.rootDirectory + path
}

func parseError(path string, err error) error {
	if s3Err, ok := err.(*s3.Error); ok && s3Err.Code == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

func hasCode(err error, code string) bool {
	s3err, ok := err.(*aws.Error)
	return ok && s3err.Code == code
}

func (d *Driver) getOptions() s3.Options {
	return s3.Options{SSE: d.Encrypt}
}

func getPermissions() s3.ACL {
	return s3.Private
}

func (d *Driver) getContentType() string {
	return "application/octet-stream"
}
