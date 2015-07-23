package storage

import (
	"errors"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
)

// Returns a list, or partial list, of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	var foundRepos []string
	var errVal error

	if len(repos) == 0 {
		return 0, errors.New("no space in slice")
	}

	root, err := defaultPathMapper.path(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	// Walk each of the directories in our storage.  Unfortunately since there's no
	// guarantee that storage will return files in lexigraphical order, we have
	// to store everything another slice, sort it and then copy it back to our
	// passed in slice.

	Walk(ctx, reg.blobStore.driver, root, func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()

		// lop the base path off
		repoPath := filePath[len(root)+1:]

		_, file := path.Split(repoPath)
		if file == "_layers" {
			repoPath = strings.TrimSuffix(repoPath, "/_layers")
			if repoPath > last {
				foundRepos = append(foundRepos, repoPath)
			}
			return ErrSkipDir
		} else if strings.HasPrefix(file, "_") {
			return ErrSkipDir
		}

		return nil
	})

	sort.Strings(foundRepos)
	n = copy(repos, foundRepos)

	// Signal that we have no more entries by setting EOF
	if len(foundRepos) <= len(repos) {
		errVal = io.EOF
	}

	return n, errVal

}
