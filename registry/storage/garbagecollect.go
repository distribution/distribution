package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

func emit(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// GCOpts contains options for garbage collector
type GCOpts struct {
	DryRun         bool
	RemoveUntagged bool
	Quiet          bool
	// Workers is the number of repositories to mark in parallel during the
	// mark phase. A value of 0 or 1 uses a single goroutine (no parallelism).
	Workers int
	// MinAge is the minimum age a blob or layer link must have before it is
	// eligible for deletion. Blobs and links modified more recently than
	// time.Now()-MinAge are preserved. Defaults to 30 days if zero.
	MinAge time.Duration
}

// ManifestDel contains manifest structure which will be deleted
type ManifestDel struct {
	Name   string
	Digest digest.Digest
	Tags   []string
}

// MarkAndSweep performs a mark and sweep of registry data
func MarkAndSweep(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts) error {
	logger := dcontext.GetLogger(ctx)
	gcStart := time.Now()

	minAge := opts.MinAge
	ageCutoff := time.Now().Add(-minAge)
	logger.Infof("garbage collection: blobs and layer links modified after %s will be preserved", ageCutoff.Format(time.RFC3339))

	// mark
	manifestMarkStart := time.Now()
	markSet := make(map[digest.Digest]struct{})
	manifestArr := make([]ManifestDel, 0)
	deleteLayerSet := make(map[string][]digest.Digest)

	var mu sync.Mutex

	// inFlight tracks which repositories are currently being marked and when
	// they started, for diagnostic dumps on SIGUSR1.
	type workerState struct {
		start time.Time
	}
	inFlight := make(map[string]workerState)
	var inFlightMu sync.Mutex

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	go func() {
		for range sigCh {
			inFlightMu.Lock()
			if len(inFlight) == 0 {
				logger.Infof("gc: no phase currently running (waiting for workers to be scheduled)")
			} else {
				logger.Infof("gc: mark phase, %d worker(s) in flight:", len(inFlight))
				for repo, state := range inFlight {
					logger.Infof("  - %s (running for %s)", repo, time.Since(state.start).Round(time.Second))
				}
			}
			inFlightMu.Unlock()
		}
	}()
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()

	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	repositoryEnumerator, ok := registry.(distribution.RepositoryEnumerator)
	if !ok {
		return fmt.Errorf("unable to convert Namespace to RepositoryEnumerator")
	}
	err := repositoryEnumerator.Enumerate(ctx, func(repoName string) error {
		// Check if the errgroup context was cancelled (i.e. a worker failed).
		if gctx.Err() != nil {
			return gctx.Err()
		}

		g.Go(func() error {
			inFlightMu.Lock()
			inFlight[repoName] = workerState{start: time.Now()}
			inFlightMu.Unlock()
			defer func() {
				inFlightMu.Lock()
				delete(inFlight, repoName)
				inFlightMu.Unlock()
			}()

			if !opts.Quiet {
				emit(repoName)
			}

			named, err := reference.WithName(repoName)
			if err != nil {
				return fmt.Errorf("failed to parse repo name %s: %v", repoName, err)
			}
			repository, err := registry.Repository(gctx, named)
			if err != nil {
				return fmt.Errorf("failed to construct repository: %v", err)
			}

			manifestService, err := repository.Manifests(gctx)
			if err != nil {
				return fmt.Errorf("failed to construct manifest service: %v", err)
			}

			manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
			if !ok {
				return fmt.Errorf("unable to convert ManifestService into ManifestEnumerator")
			}

			// Local accumulator for this repository's results; merged into
			// the shared maps under the mutex at the end.
			localMarkSet := make(map[digest.Digest]struct{})
			var localManifestArr []ManifestDel
			err = manifestEnumerator.Enumerate(gctx, func(dgst digest.Digest) error {
				if opts.RemoveUntagged {
					// fetch all tags where this manifest is the latest one
					tags, err := repository.Tags(gctx).Lookup(gctx, v1.Descriptor{Digest: dgst})
					if err != nil {
						return fmt.Errorf("failed to retrieve tags for digest %v: %v", dgst, err)
					}
					if len(tags) == 0 {
						// fetch all tags from repository
						// all of these tags could contain manifest in history
						// which means that we need check (and delete) those references when deleting manifest
						allTags, err := repository.Tags(gctx).All(gctx)
						if err != nil {
							if _, ok := err.(distribution.ErrRepositoryUnknown); ok {
								if !opts.Quiet {
									emit("manifest tags path of repository %s does not exist", repoName)
								}
								return nil
							}
							return fmt.Errorf("failed to retrieve tags %v", err)
						}
						localManifestArr = append(localManifestArr, ManifestDel{Name: repoName, Digest: dgst, Tags: allTags})
						return nil
					}
				}
				// Mark the manifest's blob
				if !opts.Quiet {
					emit("%s: marking manifest %s ", repoName, dgst)
				}
				localMarkSet[dgst] = struct{}{}

				return markManifestReferences(dgst, manifestService, gctx, func(d digest.Digest) bool {
					_, marked := localMarkSet[d]
					if !marked {
						localMarkSet[d] = struct{}{}
						if !opts.Quiet {
							emit("%s: marking blob %s", repoName, d)
						}
					}
					return marked
				})
			})

			if err != nil {
				// In certain situations such as unfinished uploads, deleting all
				// tags in S3 or removing the _manifests folder manually, this
				// error may be of type PathNotFound.
				//
				// In these cases we can continue marking other manifests safely.
				if _, ok := err.(driver.PathNotFoundError); !ok {
					return err
				}
			}

			// Enumerate layer links for this repo and check against localMarkSet.
			// Using localMarkSet is safe because layer links are repo-scoped: the
			// only question is whether this repo's own manifests reference the blob.
			blobService := repository.Blobs(gctx)
			layerEnumerator, ok := blobService.(*linkedBlobStore)
			if !ok {
				return errors.New("unable to convert BlobService into linkedBlobStore")
			}
			var deleteLayers []digest.Digest
			var skipped int
			err = layerEnumerator.EnumerateWithMeta(gctx, func(meta BlobMeta) error {
				if _, ok := localMarkSet[meta.Digest]; !ok {
					if meta.ModTime.After(ageCutoff) {
						skipped++
						return nil
					}
					deleteLayers = append(deleteLayers, meta.Digest)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to enumerate layers for %s: %v", repoName, err)
			}
			if skipped > 0 && !opts.Quiet {
				logger.Infof("%s: skipping %d layer link(s) younger than %s", repoName, skipped, minAge)
			}

			// Merge local mark results and layer deletions into shared state.
			mu.Lock()
			for d := range localMarkSet {
				markSet[d] = struct{}{}
			}
			manifestArr = append(manifestArr, localManifestArr...)
			if len(deleteLayers) > 0 {
				deleteLayerSet[repoName] = deleteLayers
			}
			mu.Unlock()

			return nil
		})

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to mark: %v", err)
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to mark: %v", err)
	}

	logger.Infof("mark phase completed in %s, %d blobs marked", time.Since(manifestMarkStart), len(markSet))

	manifestArr = unmarkReferencedManifest(manifestArr, markSet, opts.Quiet)

	// sweep
	sweepStart := time.Now()
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
	blobStoreService, ok := blobService.(*blobStore)
	if !ok {
		return errors.New("unable to convert BlobService into blobStore")
	}
	deleteSet := make(map[digest.Digest]struct{})
	var skippedBlobs int
	var totalBlobs int
	err = blobStoreService.EnumerateWithMeta(ctx, func(meta BlobMeta) error {
		totalBlobs++
		// check if digest is in markSet. If not, delete it!
		if _, ok := markSet[meta.Digest]; !ok {
			if meta.ModTime.After(ageCutoff) {
				skippedBlobs++
				return nil
			}
			deleteSet[meta.Digest] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error enumerating blobs: %v", err)
	}
	logger.Infof("%d blobs marked out of %d total blobs, %d blobs and %d manifests eligible for deletion, %d blobs skipped (too young)", len(markSet), totalBlobs, len(deleteSet), len(manifestArr), skippedBlobs)
	for dgst := range deleteSet {
		if opts.DryRun {
			continue
		}
		err = vacuum.RemoveBlob(string(dgst))
		if err != nil {
			return fmt.Errorf("failed to delete blob %s: %v", dgst, err)
		}
	}

	for repo, dgsts := range deleteLayerSet {
		for _, dgst := range dgsts {
			if !opts.Quiet {
				emit("%s: layer link eligible for deletion: %s", repo, dgst)
			}
			if opts.DryRun {
				continue
			}
			err = vacuum.RemoveLayer(repo, dgst)
			if err != nil {
				return fmt.Errorf("failed to delete layer link %s of repo %s: %v", dgst, repo, err)
			}
		}
	}

	logger.Infof("sweep phase completed in %s, %d blobs and %d manifests deleted", time.Since(sweepStart), len(deleteSet), len(manifestArr))
	logger.Infof("garbage collection completed in %s", time.Since(gcStart))

	return err
}

// unmarkReferencedManifest filters out manifest present in markSet
func unmarkReferencedManifest(manifestArr []ManifestDel, markSet map[digest.Digest]struct{}, quietOutput bool) []ManifestDel {
	filtered := make([]ManifestDel, 0)
	for _, obj := range manifestArr {
		if _, ok := markSet[obj.Digest]; !ok {
			if !quietOutput {
				emit("manifest eligible for deletion: %s", obj)
			}

			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// markManifestReferences marks the manifest references
func markManifestReferences(dgst digest.Digest, manifestService distribution.ManifestService, ctx context.Context, ingester func(digest.Digest) bool) error {
	manifest, err := manifestService.Get(ctx, dgst)
	if err != nil {
		return fmt.Errorf("failed to retrieve manifest for digest %v: %v", dgst, err)
	}

	descriptors := manifest.References()
	for _, descriptor := range descriptors {

		// do not visit references if already marked
		if ingester(descriptor.Digest) {
			continue
		}

		if ok, _ := manifestService.Exists(ctx, descriptor.Digest); ok {
			err := markManifestReferences(descriptor.Digest, manifestService, ctx, ingester)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
