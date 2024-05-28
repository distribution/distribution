package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
)

// Returns a list, or partial list, of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (reg *registry) Repositories(ctx context.Context, repos []string, last string) (int, error) {
	filledBuffer := false
	foundRepos := 0

	if len(repos) == 0 {
		return 0, errors.New("Attempted to list 0 repositories")
	}

	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return 0, err
	}

	startAfter := ""
	if last != "" {
		startAfter, err = pathFor(manifestsPathSpec{name: last})
		if err != nil {
			return 0, err
		}
	}

	err = reg.blobStore.driver.Walk(ctx, root, func(fileInfo driver.FileInfo) error {
		err := handleRepository(fileInfo, root, last, func(repoPath string) error {
			repos[foundRepos] = repoPath
			foundRepos += 1
			return nil
		})
		if err != nil {
			return err
		}

		// if we've filled our slice, no need to walk any further
		if foundRepos == len(repos) {
			filledBuffer = true
			return driver.ErrFilledBuffer
		}

		return nil
	}, driver.WithStartAfterHint(startAfter))

	if err != nil {
		return foundRepos, err
	}

	if filledBuffer {
		// There are potentially more repositories to list
		return foundRepos, nil
	}

	// We didn't fill the buffer, so that's the end of the list of repos
	return foundRepos, io.EOF
}

// Enumerate applies ingester to each repository
func (reg *registry) Enumerate(ctx context.Context, ingester func(string) error) error {
	root, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return err
	}

	err = reg.blobStore.driver.Walk(ctx, root, func(fileInfo driver.FileInfo) error {
		return handleRepository(fileInfo, root, "", ingester)
	})

	return err
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

// lessPath returns true if one path a is less than path b.
//
// A component-wise comparison is done, rather than the lexical comparison of
// strings.
func lessPath(a, b string) bool {
	// we provide this behavior by making separator always sort first.
	return compareReplaceInline(a, b, '/', '\x00') < 0
}

// compareReplaceInline modifies runtime.cmpstring to replace old with new
// during a byte-wise comparison.
func compareReplaceInline(s1, s2 string, old, new byte) int {
	// TODO(stevvooe): We are missing an optimization when the s1 and s2 have
	// the exact same slice header. It will make the code unsafe but can
	// provide some extra performance.

	l := len(s1)
	if len(s2) < l {
		l = len(s2)
	}

	for i := 0; i < l; i++ {
		c1, c2 := s1[i], s2[i]
		if c1 == old {
			c1 = new
		}

		if c2 == old {
			c2 = new
		}

		if c1 < c2 {
			return -1
		}

		if c1 > c2 {
			return +1
		}
	}

	if len(s1) < len(s2) {
		return -1
	}

	if len(s1) > len(s2) {
		return +1
	}

	return 0
}

// handleRepository calls function fn with a repository path if fileInfo
// has a path of a repository under root and that it is lexographically
// after last. Otherwise, it will return ErrSkipDir or ErrFilledBuffer.
// These should be used with Walk to do handling with repositories in a
// storage.
func handleRepository(fileInfo driver.FileInfo, root, last string, fn func(repoPath string) error) error {
	filePath := fileInfo.Path()

	// lop the base path off
	repo := filePath[len(root)+1:]

	_, file := path.Split(repo)
	if file == "_manifests" {
		repo = strings.TrimSuffix(repo, "/_manifests")
		if lessPath(last, repo) {
			if err := fn(repo); err != nil {
				return err
			}
		}
		return driver.ErrSkipDir
	} else if strings.HasPrefix(file, "_") {
		return driver.ErrSkipDir
	}

	return nil
}
