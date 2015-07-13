package storage

import (
	"path"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	storageDriver "github.com/docker/distribution/registry/storage/driver"
)

type catalogSvc struct {
	ctx    context.Context
	driver storageDriver.StorageDriver
}

var _ distribution.CatalogService = &catalogSvc{}

// Get returns a list, or partial list, of repositories in the registry.
// Because it's a quite expensive operation, it should only be used when building up
// an initial set of repositories.
func (c *catalogSvc) Get(maxEntries int, lastEntry string) ([]string, bool, error) {
	log.Infof("Retrieving up to %d entries of the catalog starting with '%s'", maxEntries, lastEntry)
	var repos []string

	root, err := defaultPathMapper.path(repositoriesRootPathSpec{})
	if err != nil {
		return repos, false, err
	}

	Walk(c.ctx, c.driver, root, func(fileInfo storageDriver.FileInfo) error {
		filePath := fileInfo.Path()

		// lop the base path off
		repoPath := filePath[len(root)+1:]

		_, file := path.Split(repoPath)
		if file == "_layers" {
			repoPath = strings.TrimSuffix(repoPath, "/_layers")
			if repoPath > lastEntry {
				repos = append(repos, repoPath)
			}
			return ErrSkipDir
		} else if strings.HasPrefix(file, "_") {
			return ErrSkipDir
		}

		return nil
	})

	sort.Strings(repos)

	moreEntries := false
	if len(repos) > maxEntries {
		moreEntries = true
		repos = repos[0:maxEntries]
	}

	return repos, moreEntries, nil
}
