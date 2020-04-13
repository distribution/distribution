package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage/driver"
)

var (
	ErrStopRec = errors.New("Stopped the recursion for getting repositories")
)

// Returns a list, or partial list, of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	var finishedWalk bool
	var foundRepos []string

	if len(repos) == 0 {
		return 0, errors.New("no space in slice")
	}

	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	err = reg.getRepositories(ctx, root, last, func(repoPath string) error {
		// this is placed before the append,
		// so that we will get a extra repo if
		// any. This assures that we do not return
		// io.EOF without it being the last record.
		if len(foundRepos) == len(repos) {
			finishedWalk = true
			return ErrStopRec
		}

		foundRepos = append(foundRepos, repoPath)

		return nil
	})

	n = copy(repos, foundRepos)

	if err != nil {
		return n, err
	} else if !finishedWalk {
		// We didn't fill buffer. No more records are available.
		return n, io.EOF
	}

	return n, err
}

// Enumerate applies ingester to each repository
func (reg *registry) Enumerate(ctx context.Context, ingester func(string) error) error {
	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return err
	}

	return reg.getRepositories(ctx, root, "", ingester)
}

// Remove removes a repository from storage
func (reg *registry) Remove(ctx context.Context, name reference.Named) error {
	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return err
	}
	repoDir := path.Join(root, name.Name())
	return reg.driver.Delete(ctx, repoDir)
}

// getRepositories is a helper function for getRepositoriesRec calls
// the function fn with a repository path, if the current path looked
// at is a repository and is lexicographically after last. It is possible
// to return driver.ErrSkipDir, if there is no interest in any repositories
// under the given `repoPath`, or call ErrStopRec if the recursion should stop.
func (reg *registry) getRepositories(ctx context.Context, root, last string, fn func(repoPath string) error) error {
	midFn := fn

	// middleware func to exclude the `last` repo
	// only use it, if there is set a last.
	if last != "" {
		midFn = func(repoPath string) error {
			if repoPath != last {
				return fn(repoPath)
			}
			return nil
		}
	}

	// call our recursive func, with the midFn and the start path
	// of where we want to find repositories.
	err := reg.getRepositoriesRec(ctx, root, root, last, midFn)
	if err == ErrStopRec {
		return nil
	}
	return err
}

// getRepositoriesRec recurse through all folders it the `lookPath`,
// there it will try to find repositories. See getRepositories for more.
func (reg *registry) getRepositoriesRec(ctx context.Context, root, lookPath, last string, fn func(repoPath string) error) error {
	// ensure that the current path is a dir, otherwise we just return
	if f, err := reg.blobStore.driver.Stat(ctx, lookPath); err != nil || !f.IsDir() {
		if err != nil {
			return err
		}
		return nil
	}

	// get children in the current path
	children, err := reg.blobStore.driver.List(ctx, lookPath)
	if err != nil {
		return err
	}

	// sort this, so that it will be added in the correct order
	sort.Strings(children)

	if last != "" {
		splitLasts := strings.Split(last, "/")

		// call the next iteration of getRepositoriesRec if any, but
		// exclude the current one.
		if len(splitLasts) > 1 {
			if err := reg.getRepositoriesRec(ctx, root, lookPath+"/"+splitLasts[0], strings.Join(splitLasts[1:], "/"), fn); err != nil {
				return err
			}
		}

		// find current last path in our children
		n := sort.SearchStrings(children, lookPath+"/"+splitLasts[0])
		if n == len(children) || children[n] != lookPath+"/"+splitLasts[0] {
			return errors.New("the provided 'last' repositories does not exists")
		}

		// if this is not a final `last` (there are more `/` left)
		// then exclude the current index, else include it
		if len(splitLasts) > 1 {
			children = children[n+1:]
		} else {
			children = children[n:]
		}
	}

	for _, child := range children {
		_, file := path.Split(child)

		if file == "_manifest" {
			if err := fn(strings.TrimPrefix(lookPath, root+"/")); err != nil {
				if err == driver.ErrSkipDir {
					break
				}
				return err
			}
		} else if file[0] != '_' {
			if err := reg.getRepositoriesRec(ctx, root, child, "", fn); err != nil {
				return err
			}
		}
	}

	return nil
}
