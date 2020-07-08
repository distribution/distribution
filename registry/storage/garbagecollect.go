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
	DryRun              bool
	RemoveUntagged      bool
	RemoveRepositories  bool
	ModificationTimeout float64
}

// ManifestDel contains manifest structure which will be deleted
type ManifestDel struct {
	Name   string
	Digest digest.Digest
	Tags   []string
}

// MarkAndSweep performs a mark and sweep of registry data
func MarkAndSweep(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts) error {

	deleteBlobSet, deleteManifestArr, deleteLayerMap, deleteRepositoryArr, err := mark(
		ctx,
		storageDriver,
		registry,
		opts.RemoveUntagged,
		opts.ModificationTimeout,
	)
	if err != nil {
		return fmt.Errorf("failed to mark blobs and manifests: %v", err)
	}

	deleteLayerCount := 0
	for k := range deleteLayerMap {
		deleteLayerCount = deleteLayerCount + len(deleteLayerMap[k])
	}

	emit(
		"\n%d blobs, %d manifests, %d layers, %d repositories eligible for deletion",
		len(deleteBlobSet),
		len(deleteManifestArr),
		deleteLayerCount,
		len(deleteRepositoryArr),
	)

	vacuum := NewVacuum(ctx, storageDriver)

	if opts.RemoveUntagged {
		err = sweepManifests(vacuum, deleteManifestArr, opts.DryRun)
		if err != nil {
			return err
		}
	}

	err = sweepLayers(vacuum, deleteLayerMap, opts.DryRun)
	if err != nil {
		return err
	}

	if opts.RemoveRepositories {
		err = sweepRepositories(vacuum, deleteRepositoryArr, opts.DryRun)
		if err != nil {
			return err
		}
	}

	err = sweepBlobs(vacuum, deleteBlobSet, opts.DryRun)
	if err != nil {
		return err
	}

	return nil
}

func sweepRepositories(vacuum Vacuum, deleteRepositoryArr []string, dryRun bool) error {
	emit("sweep repositories")
	for _, name := range deleteRepositoryArr {
		emit("repository eligible for deletion: %s", name)
		if dryRun {
			continue
		}
		err := vacuum.RemoveRepository(name)
		if err != nil {
			switch err := err.(type) {
			case driver.PathNotFoundError:
				emit("skip error: %v", err)
				continue
			default:
				return fmt.Errorf("failed to delete repository %s: %v", name, err)
			}
		}
	}
	return nil
}

func sweepLayers(vacuum Vacuum, deleteLayerMap map[string][]digest.Digest, dryRun bool) error {
	emit("sweep layers")
	for repoName, layerArr := range deleteLayerMap {
		for _, dgst := range layerArr {
			emit("layer eligible for deletion: %s %s", repoName, dgst)
			if dryRun {
				continue
			}
			err := vacuum.RemoveLayer(repoName, dgst)
			if err != nil {
				switch err := err.(type) {
				case driver.PathNotFoundError:
					emit("skip error: %v", err)
					continue
				default:
					return fmt.Errorf("failed to delete layer %s: %v", dgst, err)
				}
			}
		}
	}
	return nil
}

func sweepBlobs(vacuum Vacuum, deleteBlobSet map[digest.Digest]struct{}, dryRun bool) error {
	emit("sweep blobs")
	for dgst := range deleteBlobSet {
		emit("blob eligible for deletion: %s", dgst)
		if dryRun {
			continue
		}
		err := vacuum.RemoveBlob(string(dgst))
		if err != nil {
			switch err := err.(type) {
			case driver.PathNotFoundError:
				emit("skip error: %v", err)
				continue
			default:
				return fmt.Errorf("failed to delete blob %s: %v", dgst, err)
			}
		}
	}
	return nil
}

func sweepManifests(vacuum Vacuum, deleteManifestArr []ManifestDel, dryRun bool) error {
	emit("sweep manifests")
	for _, obj := range deleteManifestArr {
		emit("manifest eligible for deletion: %s %s", obj.Name, obj.Digest)
		if dryRun {
			continue
		}
		err := vacuum.RemoveManifest(obj.Name, obj.Digest, obj.Tags)
		if err != nil {
			switch err := err.(type) {
			case driver.PathNotFoundError:
				emit("skip error: %v", err)
				continue
			default:
				return fmt.Errorf("failed to delete manifest %s: %v", obj.Digest, err)
			}
		}
	}
	return nil
}

func mark(
	ctx context.Context,
	storageDriver driver.StorageDriver,
	registry distribution.Namespace,
	removeUntagged bool,
	modificationTimeout float64,
) (map[digest.Digest]struct{}, []ManifestDel, map[string][]digest.Digest, []string, error) {

	modTimeoutDuration := time.Duration(float64(time.Second) * modificationTimeout)
	maxModified := time.Now().Add(-modTimeoutDuration)

	repositoryEnumerator, ok := registry.(distribution.RepositoryEnumerator)
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("unable to convert Namespace to RepositoryEnumerator")
	}

	markBlobSet := make(map[digest.Digest]struct{})
	deleteManifestArr := make([]ManifestDel, 0)
	deleteRepositoryArr := make([]string, 0)
	deleteLayerMap := make(map[string][]digest.Digest)

	err := repositoryEnumerator.Enumerate(ctx, func(repoName string) error {
		emit(repoName)

		repoHasManifest := false
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

		markLayerSet := make(map[digest.Digest]struct{})
		err = manifestEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
			if removeUntagged {
				// fetch all tags where this manifest is the latest one
				tags, err := repository.Tags(ctx).Lookup(ctx, distribution.Descriptor{Digest: dgst})
				if err != nil {
					return fmt.Errorf("failed to retrieve tags for digest %v: %v", dgst, err)
				}
				if len(tags) == 0 {
					emit("manifest eligible for deletion: %s", dgst)
					// fetch all tags from repository
					// all of these tags could contain manifest in history
					// which means that we need check (and delete) those references when deleting manifest
					allTags, err := repository.Tags(ctx).All(ctx)
					if err != nil {
						return fmt.Errorf("failed to retrieve tags %v", err)
					}

					manifestModifiedEarlier := true
					// check modification
					if modificationTimeout > 0 {
						modified, err := manifestModified(ctx, storageDriver, repoName, dgst)
						if err != nil {
							return fmt.Errorf("failed to get modification time: %v", err)
						}
						if maxModified.Before(modified) {
							manifestModifiedEarlier = false
						}
					}

					if manifestModifiedEarlier {
						deleteManifestArr = append(deleteManifestArr, ManifestDel{Name: repoName, Digest: dgst, Tags: allTags})
						return nil
					}
				}
			}

			// Mark the manifest's blob
			emit("%s: marking manifest %s ", repoName, dgst)
			markBlobSet[dgst] = struct{}{}

			manifest, err := manifestService.Get(ctx, dgst)
			if err != nil {
				return fmt.Errorf("failed to retrieve manifest for digest %v: %v", dgst, err)
			}

			descriptors := manifest.References()
			for _, descriptor := range descriptors {
				markBlobSet[descriptor.Digest] = struct{}{}
				markLayerSet[descriptor.Digest] = struct{}{}
				emit("%s: marking blob %s", repoName, descriptor.Digest)
			}

			repoHasManifest = true

			return nil
		})

		// In certain situations such as unfinished uploads, deleting all
		// tags in S3 or removing the _manifests folder manually, this
		// error may be of type PathNotFound.
		//
		// In these cases we can continue marking other manifests safely.
		if _, ok := err.(driver.PathNotFoundError); ok {
			return nil
		}

		// mark layer for deletion
		blobStore := repository.Blobs(ctx)
		blobEnumerator, ok := blobStore.(distribution.BlobEnumerator)
		if !ok {
			return fmt.Errorf("unable to convert blobStore into blobEnumerator")
		}

		deleteLayerArr := make([]digest.Digest, 0)
		err = blobEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
			if _, ok := markLayerSet[dgst]; ok {
				return nil
			}
			// check modification
			if modificationTimeout > 0 {
				modified, err := layerModified(ctx, storageDriver, repoName, dgst)
				if err != nil {
					return fmt.Errorf("failed to get modification time: %v", err)
				}
				if maxModified.Before(modified) {
					return nil
				}
			}
			emit("%s: unused layer %s", repoName, dgst)
			deleteLayerArr = append(deleteLayerArr, dgst)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to get layers: %v", err)
		}
		if len(deleteLayerArr) > 0 {
			deleteLayerMap[repoName] = deleteLayerArr
		}

		// mark repo for deletion if it does not contain manifests
		if repoHasManifest {
			return nil
		}
		// check modification
		if modificationTimeout > 0 {
			modified, err := repositoryModified(ctx, storageDriver, repoName)
			if err != nil {
				return fmt.Errorf("failed to get modification time: %v", err)
			}
			if maxModified.Before(modified) {
				return nil
			}
		}
		deleteRepositoryArr = append(deleteRepositoryArr, repoName)
		emit("%s: empty repo", repoName)

		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to mark: %v", err)
	}

	blobService := registry.Blobs()
	deleteBlobSet := make(map[digest.Digest]struct{})
	err = blobService.Enumerate(ctx, func(dgst digest.Digest) error {
		// check if digest is in markSet. If not, delete it!
		if _, ok := markBlobSet[dgst]; ok {
			return nil
		}
		// check modification
		if modificationTimeout > 0 {
			modified, err := blobModified(ctx, storageDriver, dgst)
			if err != nil {
				return fmt.Errorf("failed to get modification time: %v", err)
			}
			if maxModified.Before(modified) {
				return nil
			}
		}
		deleteBlobSet[dgst] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error enumerating blobs: %v", err)
	}

	return deleteBlobSet, deleteManifestArr, deleteLayerMap, deleteRepositoryArr, nil
}

func repositoryModified(
	ctx context.Context,
	strorageDriver driver.StorageDriver,
	repoName string,
) (time.Time, error) {
	rootForRepository, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return time.Now(), err
	}
	repoDir := path.Join(rootForRepository, repoName)
	return pathModified(ctx, strorageDriver, repoDir)
}

func manifestModified(
	ctx context.Context,
	strorageDriver driver.StorageDriver,
	repoName string,
	revision digest.Digest,
) (time.Time, error) {
	return pathSpecModified(ctx, strorageDriver, manifestRevisionPathSpec{name: repoName, revision: revision})
}

func blobModified(
	ctx context.Context,
	strorageDriver driver.StorageDriver,
	digest digest.Digest,
) (time.Time, error) {
	return pathSpecModified(ctx, strorageDriver, blobPathSpec{digest: digest})
}

func layerModified(
	ctx context.Context,
	strorageDriver driver.StorageDriver,
	repoName string,
	digest digest.Digest,
) (time.Time, error) {
	return pathSpecModified(ctx, strorageDriver, layerLinkPathSpec{name: repoName, digest: digest})
}

func pathSpecModified(
	ctx context.Context,
	strorageDriver driver.StorageDriver,
	spec pathSpec,
) (time.Time, error) {
	path, err := pathFor(spec)
	if err != nil {
		return time.Now(), err
	}
	return pathModified(ctx, strorageDriver, path)
}

func pathModified(ctx context.Context, strorageDriver driver.StorageDriver, path string) (time.Time, error) {
	stat, err := strorageDriver.Stat(ctx, path)
	if err != nil {
		return time.Now(), err
	}
	return stat.ModTime(), nil
}
