package storage

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/opencontainers/go-digest"
)

func emit(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}

// GCOpts contains options for garbage collector
type GCOpts struct {
	DryRun         bool
	RemoveUntagged bool
	Parallel       int
}

// ManifestDel contains manifest structure which will be deleted
type ManifestDel struct {
	Name   string
	Digest digest.Digest
	Tags   []string
}

var manifestArr []ManifestDel
var markSet sync.Map

func mark(ctx context.Context, repoName string, registry distribution.Namespace, opts GCOpts) error {
	var err error
	emit(repoName)
	named, err := reference.WithName(repoName)
	if err != nil {
		return fmt.Errorf("failed to parse repo name %s: %v", repoName, err)
	}
	repository, err := registry.Repository(ctx, named)
	if err != nil {
		return fmt.Errorf("failed to construct repository: %v", err)
	}

	manifestService, err := repository.Manifests(ctx)
	if err != nil {
		return fmt.Errorf("failed to construct manifest service: %v", err)
	}

	manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
	if !ok {
		return fmt.Errorf("unable to convert ManifestService into ManifestEnumerator")
	}

	markBlob := func(dgst digest.Digest) error {
		// Mark the manifest's blob
		emit("%s: marking manifest %s ", repoName, dgst)
		markSet.Store(dgst, struct{}{})
		manifest, err := manifestService.Get(ctx, dgst)
		if err != nil {
			return fmt.Errorf("failed to retrieve manifest for digest %v: %v", dgst, err)
		}

		descriptors := manifest.References()
		for _, descriptor := range descriptors {
			markSet.Store(descriptor.Digest, struct{}{})
			emit("%s: marking blob %s", repoName, descriptor.Digest)
		}

		return nil
	}

	if opts.RemoveUntagged {
		digestMap := make(map[digest.Digest][]string)
		ts := repository.Tags(ctx)
		var allTags []string
		allTags, err = ts.All(ctx)
		if err != nil {
			return fmt.Errorf("failed to retrieve tags %v", err)
		}
		for _, tag := range allTags {
			descriptor, err := ts.Get(ctx, tag)
			if err != nil {
				return fmt.Errorf("failed to retrieve digest fot tag %v: %v", tag, err)
			}
			digestMap[descriptor.Digest] = append(digestMap[descriptor.Digest], tag)
		}
		err = manifestEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
			if len(digestMap[dgst]) == 0 {
				emit("manifest eligible for deletion: %s", dgst)
				// fetch all tags from repository
				// all of these tags could contain manifest in history
				// which means that we need check (and delete) those references when deleting manifest
				manifestArr = append(manifestArr, ManifestDel{Name: repoName, Digest: dgst, Tags: allTags})
				return nil
			}
			return markBlob(dgst)
		})
	} else {
		err = manifestEnumerator.Enumerate(ctx, markBlob)
	}

	// In certain situations such as unfinished uploads, deleting all
	// tags in S3 or removing the _manifests folder manually, this
	// error may be of type PathNotFound.
	//
	// In these cases we can continue marking other manifests safely.
	if _, ok := err.(driver.PathNotFoundError); ok {
		return nil
	}

	return err
}

// MarkAndSweep performs a mark and sweep of registry data
func MarkAndSweep(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts) error {
	// mark
	allRepos := make([]string, 0)
	moreRepos := true
	last := ""
	for moreRepos {
		repos := make([]string, 1024)
		n, err := registry.Repositories(ctx, repos, last)
		_, pathNotFound := err.(driver.PathNotFoundError)
		if err == io.EOF || pathNotFound {
			moreRepos = false
		} else if err != nil {
			return fmt.Errorf("failed to retrieve repos: %v", err)
		}
		if n > 0 {
			allRepos = append(allRepos, repos[:n]...)
		}
		if moreRepos {
			last = repos[n-1]
		}
	}
	if len(allRepos) == 0 {
		return nil
	}

	var err error

	for i, j := 0, 0; i < len(allRepos); i += opts.Parallel {
		var g errgroup.Group
		for j = i; j < i+opts.Parallel && j < len(allRepos); j++ {
			repoName := allRepos[j]
			g.Go(func() error {
				return mark(ctx, repoName, registry, opts)
			})
		}
		err = g.Wait()
		if err != nil {
			return fmt.Errorf("failed to mark: %v", err)
		}
	}

	// sweep
	vacuum := NewVacuum(ctx, storageDriver)
	if !opts.DryRun {
		for _, obj := range manifestArr {
			err = vacuum.RemoveManifest(obj.Name, obj.Digest, obj.Tags)
			if err != nil {
				return fmt.Errorf("failed to delete manifest %s: %v", obj.Digest, err)
			}
		}
	}
	blobService := registry.Blobs()
	deleteSet := make(map[digest.Digest]struct{})
	err = blobService.Enumerate(ctx, func(dgst digest.Digest) error {
		// check if digest is in markSet. If not, delete it!
		if _, ok := markSet.Load(dgst); !ok {
			deleteSet[dgst] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error enumerating blobs: %v", err)
	}

	length := 0
	markSet.Range(func(_, _ interface{}) bool {
		length++
		return true
	})

	emit("\n%d blobs marked, %d blobs and %d manifests eligible for deletion", length, len(deleteSet), len(manifestArr))
	for dgst := range deleteSet {
		emit("blob eligible for deletion: %s", dgst)
		if opts.DryRun {
			continue
		}
		err = vacuum.RemoveBlob(string(dgst))
		if err != nil {
			return fmt.Errorf("failed to delete blob %s: %v", dgst, err)
		}
	}

	return err
}
