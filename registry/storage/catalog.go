package storage

import (
	"errors"
	"io"
	"path"
	"strings"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
)

// Returns a list, or partial list, of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (n int, errVal error) {
	var foundRepos []string

	if len(repos) == 0 {
		return 0, errors.New("no space in slice")
	}

	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	err = WalkSortedChildren(ctx, reg.blobStore.driver, root, func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()

		// lop the base path off
		repoPath := filePath[len(root)+1:]

		_, file := path.Split(repoPath)
		if file == layersDirectory {
			repoPath = strings.TrimSuffix(repoPath, "/"+layersDirectory)
			if repoPath > last {
				foundRepos = append(foundRepos, repoPath)
			}
			return ErrSkipDir
		} else if strings.HasPrefix(file, "_") {
			return ErrSkipDir
		}

		// if we've filled our array, no need to walk any further
		if len(foundRepos) == len(repos) {
			return ErrFinishedWalk
		}

		return nil
	})

	n = copy(repos, foundRepos)

	// Signal that we have no more entries by setting EOF
	if len(foundRepos) <= len(repos) && err != ErrFinishedWalk {
		errVal = io.EOF
	}

	return n, errVal
}
