package storage

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storageDriver "github.com/docker/distribution/registry/storage/driver"
)

var (
	reTarsumPrefix = regexp.MustCompile(`^tarsum(?:/(\w+))?`)
	reDigestPath   = regexp.MustCompile(fmt.Sprintf(`^([^/]+)/(?:\w{%d}/)?(\w+)$`, multilevelHexPrefixLength))
)

// ErrSkipDir is used as a return value from onFileFunc to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var ErrSkipDir = errors.New("skip this directory")

// ErrFinishedWalk is used when the called walk function no longer wants
// to accept any more values.  This is used for pagination when the
// required number of items have been found.
var ErrFinishedWalk = errors.New("finished walk")

// WalkFn is called once per file by Walk
// If the returned error is ErrSkipDir and fileInfo refers
// to a directory, the directory will not be entered and Walk
// will continue the traversal.  Otherwise Walk will return
type WalkFn func(fileInfo storageDriver.FileInfo) error

// WalkChildrenFilter transforms a list of directory children during a
// walk before before it's recursively traversed.
type WalkChildrenFilter func([]string) []string

// walkChildrenSortedFilter causes Walk to process entries in a lexicographical
// order.
func walkChildrenSortedFilter(children []string) []string {
	sort.Stable(sort.StringSlice(children))
	return children
}

// walkChildrenNoFilter is an identity filter for directory children.
func walkChildrenNoFilter(children []string) []string {
	return children
}

// WalkWithChildrenFilter traverses a filesystem defined within driver,
// starting from the given path, calling f on each file. Given filter will be
// called on a list of directory children before beeing recursively processed.
func WalkWithChildrenFilter(ctx context.Context, driver storageDriver.StorageDriver, from string, filter WalkChildrenFilter, f WalkFn) error {
	children, err := driver.List(ctx, from)
	if err != nil {
		return err
	}
	filter(children)
	for _, child := range children {
		// TODO(stevvooe): Calling driver.Stat for every entry is quite
		// expensive when running against backends with a slow Stat
		// implementation, such as s3. This is very likely a serious
		// performance bottleneck.
		fileInfo, err := driver.Stat(ctx, child)
		if err != nil {
			return err
		}
		err = f(fileInfo)
		skipDir := (err == ErrSkipDir)
		if err != nil && !skipDir {
			return err
		}

		if fileInfo.IsDir() && !skipDir {
			if err := WalkWithChildrenFilter(ctx, driver, child, filter, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file.
func Walk(ctx context.Context, driver storageDriver.StorageDriver, from string, f WalkFn) error {
	return WalkWithChildrenFilter(ctx, driver, from, walkChildrenNoFilter, f)
}

// WalkSortedChildren traverses a filesystem defined within driver, starting
// from the given path, calling f on each file in lexicographical order.
func WalkSortedChildren(ctx context.Context, driver storageDriver.StorageDriver, from string, f WalkFn) error {
	return WalkWithChildrenFilter(ctx, driver, from, walkChildrenSortedFilter, f)
}

// pushError formats an error type given a path and an error
// and pushes it to a slice of errors
func pushError(errors []error, path string, err error) []error {
	return append(errors, fmt.Errorf("%s: %s", path, err))
}

// makeBlobStoreWalkFunc returns a function for walking a blob store at
// particular rootPath. The returned function calls a given ingest callback on
// each digest found. The blob store is expected to have following layout:
//
//     if multilevel is true:
//       <rootPath>/<alg>/<prefix>/<digest>
//       <rootPath>/tarsum/<version>/<alg>/<prefix>/<digest>
//     otherwise:
//       <rootPath>/<alg>/<digest>
//       <rootPath>/tarsum/<version>/<alg>/<digest>
func makeBlobStoreWalkFunc(rootPath string, multilevel bool, ingest func(digest.Digest) error) (WalkFn, error) {
	var (
		// number of slashes in a path to a full digest directory under a rootPath
		blobRefPathSepCount       int
		blobTarsumRefPathSepCount int
	)

	if multilevel {
		// <alg>/<prefix>/<digest>
		blobRefPathSepCount = 2
		// tarsum/<version>/<alg>/<prefix>/<digest>
		blobTarsumRefPathSepCount = 4
	} else {
		// <alg>/<digest>
		blobRefPathSepCount = 1
		// tarsum/<version>/<alg>/<digest>
		blobTarsumRefPathSepCount = 3
	}

	return func(fi storageDriver.FileInfo) error {
		if !fi.IsDir() {
			// ignore files
			return nil
		}

		// trim <from>/ prefix
		pth := strings.TrimPrefix(strings.TrimPrefix(fi.Path(), rootPath), "/")
		sepCount := strings.Count(pth, "/")

		if sepCount < blobRefPathSepCount {
			// don't try to process short paths
			return nil
		}

		alg := ""
		tarsumParts := reTarsumPrefix.FindStringSubmatch(pth)
		isTarsum := len(tarsumParts) > 0
		if sepCount > blobTarsumRefPathSepCount || (!isTarsum && sepCount > blobRefPathSepCount) {
			// too many path components
			return ErrSkipDir
		}

		if len(tarsumParts) > 0 {
			alg = "tarsum." + tarsumParts[1] + "+"
			// trim "tarsum/<version>/" prefix from path
			pth = strings.TrimPrefix(pth[len(tarsumParts[0]):], "/")
		}

		digestParts := reDigestPath.FindStringSubmatch(pth)
		if len(digestParts) > 0 {
			alg += digestParts[1]
			dgstHex := digestParts[2]
			dgst := digest.NewDigestFromHex(alg, dgstHex)
			// append only valid digests
			if err := dgst.Validate(); err == nil {
				err := ingest(dgst)
				if err != nil {
					return ErrFinishedWalk
				}
			}
			return ErrSkipDir
		}

		return nil
	}, nil
}

// enumerateAllBlobs is a utility function that returns all the blob digests
// found in given blob store. It should be used with care because of memory and
// time complexity.
func enumerateAllBlobs(be distribution.BlobEnumerator, ctx context.Context) ([]digest.Digest, error) {
	res := []digest.Digest{}
	err := be.Enumerate(ctx, func(dgst digest.Digest) error {
		res = append(res, dgst)
		return nil
	})
	return res, err
}
