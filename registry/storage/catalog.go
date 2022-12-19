package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage/driver"
)

var (
	// ErrStopReposWalk is used as a return value to indicate that the repository path walk
	// should be stopped. It's not returned as an error by any function.
	ErrStopReposWalk = errors.New("stop repos walk")
)

// Returns a list or a partial list of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	var finishedWalk bool
	var foundRepos []string

	if len(repos) == 0 {
		return -1, errors.New("no repos requested")
	}

	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	err = reg.walkRepos(ctx, root, last, func(repoPath string) error {
		// this is placed before the append,
		// so that we will get an extra repo if
		// any. This ensures that we do not return
		// io.EOF without it being the last record.
		if len(foundRepos) == len(repos) {
			finishedWalk = true
			return ErrStopReposWalk
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

	return reg.walkRepos(ctx, root, "", ingester)
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

// walkRepos walks paths rooted in root calling fn for each found repo path.
// Paths are walked in a lexical order which makes the output deterministic.
// If last is not an empty string it walks all repo paths. Otherwise
// it returns when last repoPath is found.
func (reg *registry) walkRepos(ctx context.Context, root, last string, fn func(repoPath string) error) error {
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
	err := reg.walkReposPath(ctx, root, root, last, midFn)
	if err == ErrStopReposWalk {
		return nil
	}
	return err
}

// walkReposPath walks through all folders in `lookPath`,
// looking for repositories. See walkRepos for more detailed description.
func (reg *registry) walkReposPath(ctx context.Context, root, lookPath, last string, fn func(repoPath string) error) error {
	// get children in the current path
	children, err := reg.blobStore.driver.List(ctx, lookPath)
	if err != nil {
		return err
	}

	// sort this, so that it will be added in the correct order
	sort.Strings(children)

	if last != "" {
		splitLast := strings.Split(last, "/")

		// call the next iteration of walkReposPath if any, but
		// exclude the current one.
		if len(splitLast) > 1 {
			if err := reg.walkReposPath(ctx, root, lookPath+"/"+splitLast[0], strings.Join(splitLast[1:], "/"), fn); err != nil {
				return err
			}
		}

		// find current last path in our children
		n := sort.SearchStrings(children, lookPath+"/"+splitLast[0])
		if n == len(children) || children[n] != lookPath+"/"+splitLast[0] {
			return fmt.Errorf("%q repository not found", last)
		}

		// if this is not a final `last` (there are more `/` left)
		// then exclude the current index, else include it
		if len(splitLast) > 1 {
			children = children[n+1:]
		} else {
			children = children[n:]
		}
	}

	for _, child := range children {
		_, file := path.Split(child)

		if file == "_manifests" {
			if err := fn(strings.TrimPrefix(lookPath, root+"/")); err != nil {
				if err == driver.ErrSkipDir {
					break
				}
				return err
			}
		} else if !strings.HasPrefix(file, "_") {
			if err := reg.walkReposPath(ctx, root, child, "", fn); err != nil {
				return err
			}
		}
	}

	return nil
}
