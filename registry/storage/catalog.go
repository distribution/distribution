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
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	var foundRepos []string

	if len(repos) == 0 {
		return 0, errors.New("no space in slice")
	}

	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	// errFinishedWalk signals an early exit to the walk when the current query
	// is satisfied.
	errFinishedWalk := errors.New("finished walk")

	err = Walk(ctx, reg.blobStore.driver, root, func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()

		// lop the base path off
		repoPath := filePath[len(root)+1:]

		_, file := path.Split(repoPath)
		if file == "_layers" {
			repoPath = strings.TrimSuffix(repoPath, "/_layers")
			if pathGreaterThan(repoPath, last) {
				foundRepos = append(foundRepos, repoPath)
			}
			return ErrSkipDir
		} else if strings.HasPrefix(file, "_") {
			return ErrSkipDir
		}

		// if we've filled our array, no need to walk any further
		if len(foundRepos) == len(repos) {
			return errFinishedWalk
		}

		return nil
	})

	n = copy(repos, foundRepos)

	switch err {
	case nil:
		// nil means that we completed walk and didn't fill buffer. No more
		// records are available.
		err = io.EOF
	case errFinishedWalk:
		// more records are available.
		err = nil
	}

	return n, err
}

// Enumerate applies ingester to each repository
func (reg *registry) Enumerate(ctx context.Context, ingester func(string) error) error {
	repoNameBuffer := make([]string, 100)
	var last string
	for {
		n, err := reg.Repositories(ctx, repoNameBuffer, last)
		if err != nil && err != io.EOF {
			return err
		}

		if n == 0 {
			break
		}

		last = repoNameBuffer[n-1]
		for i := 0; i < n; i++ {
			repoName := repoNameBuffer[i]
			err = ingester(repoName)
			if err != nil {
				return err
			}
		}

		if err == io.EOF {
			break
		}
	}
	return nil

}

func pathGreaterThan(pathX, pathY string) (b bool) {
	splitPathX := strings.SplitN(pathX, "/", 2)
	splitPathY := strings.SplitN(pathY, "/", 2)

	if splitPathX[0] == splitPathY[0] {
		if len(splitPathX) == 1 && len(splitPathY) == 1 {
			return false
		} else if len(splitPathX) == 1 && len(splitPathY) != 1 {
			return false
		} else if len(splitPathX) != 1 && len(splitPathY) == 1 {
			return true
		}

		return pathGreaterThan(splitPathX[1], splitPathY[1])

	}

	return splitPathX[0] > splitPathY[0]
}
