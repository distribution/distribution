package driver

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// ErrSkipDir is used as a return value from onFileFunc to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var ErrSkipDir = errors.New("skip this directory")

// ErrFilledBuffer is used as a return value from onFileFunc to indicate
// that the requested number of entries has been reached and the walk can
// stop.
var ErrFilledBuffer = errors.New("we have enough entries")

// WalkFn is called once per file by Walk
type WalkFn func(fileInfo FileInfo) error

// WalkFallback traverses a filesystem defined within driver, starting
// from the given path, calling f on each file. It uses the List method and Stat to drive itself.
// If the returned error from the WalkFn is ErrSkipDir the directory will not be entered and Walk
// will continue the traversal. If the returned error from the WalkFn is ErrFilledBuffer, the walk
// stops.
func WalkFallback(ctx context.Context, driver StorageDriver, from string, f WalkFn, options ...func(*WalkOptions)) error {
	walkOptions := &WalkOptions{}
	for _, o := range options {
		o(walkOptions)
	}

	startAfterHint := walkOptions.StartAfterHint
	// Ensure that we are checking the hint is contained within from by adding a "/".
	// Add to both in case the hint and form are the same, which would still count.
	rel, err := filepath.Rel(from, startAfterHint)
	if err != nil || strings.HasPrefix(rel, "..") {
		// The startAfterHint is outside from, so check if we even need to walk anything
		// Replace any path separators with \x00 so that the sort works in a depth-first way
		if strings.ReplaceAll(startAfterHint, "/", "\x00") < strings.ReplaceAll(from, "/", "\x00") {
			_, err := doWalkFallback(ctx, driver, from, "", f)
			return err
		}
	} else {
		// The startAfterHint is within from.
		// Walk up the tree until we hit from - we know it is contained.
		// Ensure startAfterHint is never deeper than a child of the base
		// directory so that doWalkFallback doesn't have to worry about
		// depth-first comparisons
		base := startAfterHint
		for strings.HasPrefix(base, from) {
			_, err = doWalkFallback(ctx, driver, base, startAfterHint, f)
			switch err.(type) {
			case nil:
				// No error
			case PathNotFoundError:
				// dir doesn't exist, so nothing to walk
			default:
				return err
			}
			if base == from {
				break
			}
			startAfterHint = base
			base, _ = filepath.Split(startAfterHint)
			if len(base) > 1 {
				base = strings.TrimSuffix(base, "/")
			}
		}
	}

	return nil
}

// doWalkFallback performs a depth first walk using recursion.
// from is the directory that this iteration of the function should walk.
// startAfterHint is the child within from to start the walk after. It should only ever be a child of from, or the empty string.
func doWalkFallback(ctx context.Context, driver StorageDriver, from string, startAfterHint string, f WalkFn) (bool, error) {
	children, err := driver.List(ctx, from)
	if err != nil {
		return false, err
	}
	sort.Strings(children)
	for _, child := range children {
		// The startAfterHint has been sanitised in WalkFallback and will either be
		// empty, or be suitable for an <= check for this _from_.
		if child <= startAfterHint {
			continue
		}

		// TODO(stevvooe): Calling driver.Stat for every entry is quite
		// expensive when running against backends with a slow Stat
		// implementation, such as GCS. This is very likely a serious
		// performance bottleneck.
		// Those backends should have custom walk functions. See S3.
		fileInfo, err := driver.Stat(ctx, child)
		if err != nil {
			switch err.(type) {
			case PathNotFoundError:
				// repository was removed in between listing and enumeration. Ignore it.
				logrus.WithField("path", child).Infof("ignoring deleted path")
				continue
			default:
				return false, err
			}
		}
		err = f(fileInfo)
		if err == nil && fileInfo.IsDir() {
			if ok, err := doWalkFallback(ctx, driver, child, startAfterHint, f); err != nil || !ok {
				return ok, err
			}
		} else if err == ErrSkipDir {
			// don't traverse into this directory
		} else if err == ErrFilledBuffer {
			return false, nil // no error but stop iteration
		} else if err != nil {
			return false, err
		}
	}
	return true, nil
}
