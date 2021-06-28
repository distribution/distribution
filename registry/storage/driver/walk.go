package driver

import (
	"context"
	"errors"
	"sort"

	"github.com/sirupsen/logrus"
)

// ErrSkipDir is used as a return value from onFileFunc to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var ErrSkipDir = errors.New("skip this directory")

// WalkFn is called once per file by Walk
type WalkFn func(fileInfo FileInfo) error

// WalkFallback traverses a filesystem defined within driver, starting
// from the given path, calling f on each file. It uses the List method and Stat to drive itself.
// If the returned error from the WalkFn is ErrSkipDir and fileInfo refers
// to a directory, the directory will not be entered and Walk
// will continue the traversal.  If fileInfo refers to a normal file, processing stops
func WalkFallback(ctx context.Context, driver StorageDriver, from string, f WalkFn) error {
	err, _ := doWalkFallback(ctx, driver, from, f)
	return err
}

func doWalkFallback(ctx context.Context, driver StorageDriver, from string, f WalkFn) (error, bool) {
	children, err := driver.List(ctx, from)
	if err != nil {
		return err, false
	}
	sort.Stable(sort.StringSlice(children))
	for _, child := range children {
		// TODO(stevvooe): Calling driver.Stat for every entry is quite
		// expensive when running against backends with a slow Stat
		// implementation, such as s3. This is very likely a serious
		// performance bottleneck.
		fileInfo, err := driver.Stat(ctx, child)
		if err != nil {
			switch err.(type) {
			case PathNotFoundError:
				// repository was removed in between listing and enumeration. Ignore it.
				logrus.WithField("path", child).Infof("ignoring deleted path")
				continue
			default:
				return err, false
			}
		}
		err = f(fileInfo)
		if err == nil && fileInfo.IsDir() {
			if err, ok := doWalkFallback(ctx, driver, child, f); err != nil || !ok {
				return err, ok
			}
		} else if err == ErrSkipDir {
			// noop for folders, will just skip
			if !fileInfo.IsDir() {
				return nil, false // no error but stop iteration
			}
		} else if err != nil {
			return err, false
		}
	}
	return nil, true
}
