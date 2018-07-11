package storage

import (
	"context"
	"fmt"
	"path"
	"time"

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
}

// ManifestDel contains manifest structure which will be deleted
type ManifestDel struct {
	Name   string
	Digest digest.Digest
	Tags   []string
}

// MarkAndSweep performs a mark and sweep of registry data
func MarkAndSweep(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts) error {
	repositoryEnumerator, ok := registry.(distribution.RepositoryEnumerator)
	if !ok {
		return fmt.Errorf("unable to convert Namespace to RepositoryEnumerator")
	}
	emit("GC mark phase %v", time.Now().String())

	// mark
	markSet := make(map[digest.Digest]struct{})
	manifestArr := make([]ManifestDel, 0)
	err := repositoryEnumerator.Enumerate(ctx, func(repoName string) error {
		emit(repoName)

		var err error
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

		// 1. fetch all tag names
		allTags, err := repository.Tags(ctx).All(ctx)
		switch err.(type) {
		case distribution.ErrRepositoryUnknown:
			break
		case nil:
			break
		default:
			return fmt.Errorf("failed to retrieve tags %v", err)
		}

		digestUsed := make(map[digest.Digest]int)
		tagDigests := make(map[string][]digest.Digest)

		// 2. read each tag's _current_ digest and mark its usage; store all index links for later reference
		for _, tag := range allTags {
			description, err := repository.Tags(ctx).Get(ctx, tag)
			switch err.(type) {
			case distribution.ErrTagUnknown:
				// corrupted storage; current link is missing
				break
			case nil:
				digestUsed[description.Digest] = 1
				break
			default:
				return fmt.Errorf("failed to retrieve tag %v: %v", tag, err)
			}

			// tag links (historical and current)
			digests, err := getDigests(ctx, storageDriver, repoName, tag)
			if err != nil {
				return fmt.Errorf("failed to retrieve tag links %v: %v", tag, err)
			}

			if digests != nil {
				tagDigests[tag] = digests
			}
		}

		// 3. produce digest usage map by transposing tagsDigests
		digestTags := make(map[digest.Digest][]string)
		for tag, digests := range tagDigests {
			for _, digest := range digests {
				digestTags[digest] = append(digestTags[digest], tag)
			}
		}

		err = manifestEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
			if opts.RemoveUntagged {
				// check if this digest is used by any tag
				if _, exists := digestUsed[dgst]; !exists {
					emit("manifest eligible for deletion: %s", dgst)
					// add only tags linking to given digest
					manifestArr = append(manifestArr, ManifestDel{Name: repoName, Digest: dgst, Tags: digestTags[dgst]})
					return nil
				}
			}
			// Mark the manifest's blob
			emit("%s: marking manifest %s ", repoName, dgst)
			markSet[dgst] = struct{}{}

			manifest, err := manifestService.Get(ctx, dgst)
			if err != nil {
				return fmt.Errorf("failed to retrieve manifest for digest %v: %v", dgst, err)
			}

			descriptors := manifest.References()
			for _, descriptor := range descriptors {
				markSet[descriptor.Digest] = struct{}{}
				emit("%s: marking blob %s", repoName, descriptor.Digest)
			}

			return nil
		})

		if err != nil {
			// In certain situations such as unfinished uploads, deleting all
			// tags in S3 or removing the _manifests folder manually, this
			// error may be of type PathNotFound.
			//
			// In these cases we can continue marking other manifests safely.
			if _, ok := err.(driver.PathNotFoundError); ok {
				return nil
			}
		}

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to mark: %v", err)
	}

	// sweep
	vacuum := NewVacuum(ctx, storageDriver)
	if !opts.DryRun {
		emit("GC manifest removal phase %v", time.Now().String())
		for _, obj := range manifestArr {
			err = vacuum.RemoveManifest(obj.Name, obj.Digest, obj.Tags)
			if err != nil {
				return fmt.Errorf("failed to delete manifest %s: %v", obj.Digest, err)
			}
		}
	}

	emit("GC blob scan phase %v", time.Now().String())
	blobService := registry.Blobs()
	deleteSet := make(map[digest.Digest]struct{})
	err = blobService.Enumerate(ctx, func(dgst digest.Digest) error {
		// check if digest is in markSet. If not, delete it!
		if _, ok := markSet[dgst]; !ok {
			deleteSet[dgst] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error enumerating blobs: %v", err)
	}
	emit("\n%d blobs marked, %d blobs and %d manifests eligible for deletion", len(markSet), len(deleteSet), len(manifestArr))
	emit("GC blob removal phase %v", time.Now().String())
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
	emit("GC done %v", time.Now().String())

	return err
}

// Finds all digests given tag links to
func getDigests(ctx context.Context, storageDriver driver.StorageDriver, repoName string, tag string) ([]digest.Digest, error) {
	indexPath, err := pathFor(manifestTagIndexPathSpec{
		name: repoName,
		tag:  tag,
	})

	if err != nil {
		return nil, err
	}

	var descriptors []digest.Digest

	err = storageDriver.Walk(ctx, indexPath, func(fileInfo driver.FileInfo) error {
		if fileInfo.IsDir() {
			return nil
		}

		filePath := fileInfo.Path()

		dir, fileName := path.Split(filePath)
		if fileName != "link" {
			return nil
		}

		digest, err := digestFromLinkDir(dir)
		if err != nil {
			return err
		}

		descriptors = append(descriptors, digest)
		return nil
	})

	if err != nil {
		if _, ok := err.(driver.PathNotFoundError); ok {
			return descriptors, nil
		}

		return nil, fmt.Errorf("failed to read tags %v digests: %v", tag, err)
	}

	return descriptors, nil
}

// Reconstructs a digest from a link directory
func digestFromLinkDir(dir string) (digest.Digest, error) {
	dir = path.Dir(dir)
	dir, hex := path.Split(dir)
	dir = path.Dir(dir)
	dir, algo := path.Split(dir)

	dgst := digest.NewDigestFromHex(algo, hex)
	return dgst, dgst.Validate()
}
