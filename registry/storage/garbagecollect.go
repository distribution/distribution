package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"
)

func emit(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// GCOpts contains options for garbage collector
type GCOpts struct {
	DryRun           bool
	RemoveUntagged   bool
	Quiet            bool
	MaxConcurrency   int           // default: 4
	ProgressInterval time.Duration // default: 30s
	CheckpointDir    string        // optional: enable checkpointing
	Timeout          time.Duration // default: 24h
	MarkOnly         bool          // only run mark phase, save candidates
	SweepOnly        bool          // only run sweep phase from checkpoint
}

// CheckpointState represents the saved state for resume capability
type CheckpointState struct {
	Version            string    `json:"version"`
	Timestamp          time.Time `json:"timestamp"`
	MarkPhaseComplete  bool      `json:"mark_phase_complete"`
	Stats              GCStats   `json:"stats"`
	DeletionCandidates []string  `json:"deletion_candidates"` // blob digests
}

// LockFile represents the distributed lock
type LockFile struct {
	Hostname  string    `json:"hostname"`
	PID       int       `json:"pid"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   string    `json:"timeout"`
}

// GCStats contains statistics about garbage collection
type GCStats struct {
	// Repositories
	ReposProcessed int
	ReposTotal     int

	// Mark phase
	ManifestsMarked   int
	BlobsMarked       int
	MarkDuration      time.Duration // Phase 1/2: marking referenced
	BlobEnumDuration  time.Duration // Phase 2/2: blob enumeration
	TotalMarkDuration time.Duration // Phase 1/2 + 2/2 combined

	// Sweep phase
	ManifestsDeleted  int
	BlobsDeleted      int
	LayerLinksDeleted int
	BytesDeleted      int64 // Total bytes freed
	SweepDuration     time.Duration

	// Overall
	TotalDuration time.Duration
	Errors        []error
}

// ManifestDel contains manifest structure which will be deleted
type ManifestDel struct {
	Name   string
	Digest digest.Digest
	Tags   []string
}

// MarkAndSweep performs a mark and sweep of registry data
func MarkAndSweep(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts) error {
	// Set defaults
	if opts.MaxConcurrency == 0 {
		opts.MaxConcurrency = 4
	}
	if opts.ProgressInterval == 0 {
		opts.ProgressInterval = 30 * time.Second
	}
	if opts.Timeout == 0 {
		opts.Timeout = 24 * time.Hour
	}

	// Validate options
	if opts.MarkOnly && opts.SweepOnly {
		return fmt.Errorf("cannot specify both --mark-only and --sweep")
	}
	if opts.SweepOnly && opts.CheckpointDir == "" {
		return fmt.Errorf("--sweep requires --checkpoint-dir to load candidates")
	}
	if opts.MarkOnly && opts.CheckpointDir == "" {
		return fmt.Errorf("--mark-only requires --checkpoint-dir to save candidates")
	}

	// Acquire distributed lock if using checkpoint dir
	if opts.CheckpointDir != "" {
		if err := acquireLock(opts.CheckpointDir, opts.Timeout); err != nil {
			return fmt.Errorf("failed to acquire lock: %v", err)
		}
		defer releaseLock(opts.CheckpointDir)
	}

	// Create context with timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Initialize stats
	stats := &GCStats{}
	startTime := time.Now()
	logger := dcontext.GetLogger(ctx)

	mode := "full"
	if opts.MarkOnly {
		mode = "mark-only"
	} else if opts.SweepOnly {
		mode = "sweep-only"
	}

	logger.Infof("Starting garbage collection (mode=%s, timeout=%v, workers=%d)",
		mode, opts.Timeout, opts.MaxConcurrency)

	// Run mark and sweep with stats tracking
	err := markAndSweepWithStats(ctx, storageDriver, registry, opts, stats, logger)

	stats.TotalDuration = time.Since(startTime)

	// Log final summary
	if !opts.Quiet {
		logger.Infof("GC complete: mode=%s total_time=%v mark_time=%v (mark_refs=%v enum_blobs=%v) sweep_time=%v "+
			"repos=%d manifests_marked=%d blobs_marked=%d "+
			"manifests_deleted=%d blobs_deleted=%d space_reclaimed=%s layer_links_deleted=%d errors=%d",
			mode, stats.TotalDuration, stats.TotalMarkDuration, stats.MarkDuration, stats.BlobEnumDuration, stats.SweepDuration,
			stats.ReposProcessed, stats.ManifestsMarked, stats.BlobsMarked,
			stats.ManifestsDeleted, stats.BlobsDeleted, humanizeBytes(stats.BytesDeleted),
			stats.LayerLinksDeleted, len(stats.Errors))
	}

	return err
}

// markAndSweepWithStats performs mark and sweep with detailed progress tracking
func markAndSweepWithStats(ctx context.Context, storageDriver driver.StorageDriver, registry distribution.Namespace, opts GCOpts, stats *GCStats, logger dcontext.Logger) error {
	repositoryEnumerator, ok := registry.(distribution.RepositoryEnumerator)
	if !ok {
		return fmt.Errorf("unable to convert Namespace to RepositoryEnumerator")
	}

	// Load checkpoint if in sweep-only mode
	var loadedCandidates map[digest.Digest]struct{}
	if opts.SweepOnly {
		checkpoint, err := loadCheckpoint(opts.CheckpointDir)
		if err != nil {
			return fmt.Errorf("failed to load checkpoint: %v", err)
		}
		if checkpoint == nil {
			return fmt.Errorf("no checkpoint found, run mark phase first")
		}

		logger.Infof("Loaded checkpoint from %v with %d deletion candidates",
			checkpoint.Timestamp, len(checkpoint.DeletionCandidates))

		// Convert candidates to map for later filtering
		loadedCandidates = make(map[digest.Digest]struct{})
		for _, candidate := range checkpoint.DeletionCandidates {
			dgst, err := digest.Parse(candidate)
			if err != nil {
				logger.Warnf("Invalid digest in checkpoint: %s", candidate)
				continue
			}
			loadedCandidates[dgst] = struct{}{}
		}

		logger.Info("Re-running mark phase to catch new references")
	}

	// Mark phase
	markStart := time.Now()
	logger.Info("Starting mark phase (1/2: marking referenced blobs)")

	// Shared data structures with mutex protection for concurrent access
	var markSetMu sync.Mutex
	markSet := make(map[digest.Digest]struct{})

	var deleteLayerSetMu sync.Mutex
	deleteLayerSet := make(map[string][]digest.Digest)

	var manifestArrMu sync.Mutex
	manifestArr := make([]ManifestDel, 0)

	// Progress tracking
	var statsMu sync.Mutex
	lastProgress := time.Now()
	progressTicker := time.NewTicker(opts.ProgressInterval)
	defer progressTicker.Stop()

	// Collect all repository names first
	var repoNames []string
	err := repositoryEnumerator.Enumerate(ctx, func(repoName string) error {
		repoNames = append(repoNames, repoName)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to enumerate repositories: %v", err)
	}

	// Process repositories in parallel using worker pool
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(opts.MaxConcurrency)

	for _, repoName := range repoNames {
		g.Go(func() error {
			// Check for context cancellation
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			default:
			}

			// Thread-safe stats update
			statsMu.Lock()
			stats.ReposProcessed++
			// Progress reporting (always show, even in quiet mode)
			if time.Since(lastProgress) >= opts.ProgressInterval {
				elapsed := time.Since(markStart)
				manifestRate := float64(stats.ManifestsMarked) / elapsed.Seconds()
				logger.Infof("Mark progress (1/2: marking referenced): repos=%d manifests=%d blobs=%d (elapsed=%v, rate=%.1f manifests/sec)",
					stats.ReposProcessed, stats.ManifestsMarked, stats.BlobsMarked,
					elapsed, manifestRate)
				lastProgress = time.Now()
			}
			statsMu.Unlock()

			if !opts.Quiet {
				emit(repoName)
			}

			var err error
			named, err := reference.WithName(repoName)
			if err != nil {
				return fmt.Errorf("failed to parse repo name %s: %v", repoName, err)
			}
			repository, err := registry.Repository(groupCtx, named)
			if err != nil {
				return fmt.Errorf("failed to construct repository: %v", err)
			}

			manifestService, err := repository.Manifests(groupCtx)
			if err != nil {
				return fmt.Errorf("failed to construct manifest service: %v", err)
			}

			manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
			if !ok {
				return fmt.Errorf("unable to convert ManifestService into ManifestEnumerator")
			}

			// Optimization: When RemoveUntagged is enabled, fetch all tags once per repo
			// and build a digest-to-tags map for fast in-memory lookups
			var allTags []string
			var tagToDigestMap map[string]digest.Digest
			if opts.RemoveUntagged {
				var err error
				allTags, err = repository.Tags(groupCtx).All(groupCtx)
				if err != nil {
					if _, ok := err.(distribution.ErrRepositoryUnknown); !ok {
						return fmt.Errorf("failed to retrieve all tags for repo %s: %v", repoName, err)
					}
					// Repository has no tags, continue with empty tag list
					allTags = []string{}
				}

				// Build map of tag -> latest digest for fast lookups
				tagToDigestMap = make(map[string]digest.Digest, len(allTags))
				for _, tag := range allTags {
					desc, err := repository.Tags(groupCtx).Get(groupCtx, tag)
					if err != nil {
						// Tag might have been deleted concurrently, skip it
						continue
					}
					tagToDigestMap[tag] = desc.Digest
				}
			}

			err = manifestEnumerator.Enumerate(groupCtx, func(dgst digest.Digest) error {
				if opts.RemoveUntagged {
					// Check if this manifest digest is the current target of any tag
					isTagged := false
					for _, tagDigest := range tagToDigestMap {
						if tagDigest == dgst {
							isTagged = true
							break
						}
					}

					if !isTagged {
						// Manifest is untagged, add to deletion candidates
						manifestArrMu.Lock()
						manifestArr = append(manifestArr, ManifestDel{Name: repoName, Digest: dgst, Tags: allTags})
						manifestArrMu.Unlock()
						return nil
					}
				}
				// Mark the manifest's blob
				if !opts.Quiet {
					emit("%s: marking manifest %s ", repoName, dgst)
				}

				markSetMu.Lock()
				markSet[dgst] = struct{}{}
				markSetMu.Unlock()

				statsMu.Lock()
				stats.ManifestsMarked++
				statsMu.Unlock()

				return markManifestReferences(dgst, manifestService, groupCtx, func(d digest.Digest) bool {
					markSetMu.Lock()
					defer markSetMu.Unlock()

					_, marked := markSet[d]
					if !marked {
						markSet[d] = struct{}{}
						statsMu.Lock()
						stats.BlobsMarked++
						statsMu.Unlock()
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

			blobService := repository.Blobs(groupCtx)
			layerEnumerator, ok := blobService.(distribution.ManifestEnumerator)
			if !ok {
				return errors.New("unable to convert BlobService into ManifestEnumerator")
			}

			var deleteLayers []digest.Digest
			err = layerEnumerator.Enumerate(groupCtx, func(dgst digest.Digest) error {
				markSetMu.Lock()
				_, exists := markSet[dgst]
				markSetMu.Unlock()

				if !exists {
					deleteLayers = append(deleteLayers, dgst)
				}
				return nil
			})

			if len(deleteLayers) > 0 {
				deleteLayerSetMu.Lock()
				deleteLayerSet[repoName] = deleteLayers
				deleteLayerSetMu.Unlock()
			}
			return err
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to mark: %v", err)
	}

	stats.MarkDuration = time.Since(markStart)
	logger.Infof("Mark phase (1/2: marking referenced) complete: repos=%d manifests=%d blobs=%d duration=%v",
		stats.ReposProcessed, stats.ManifestsMarked, stats.BlobsMarked, stats.MarkDuration)

	manifestArr = unmarkReferencedManifest(manifestArr, markSet, opts.Quiet)

	// Blob enumeration - optimization for sweep-only mode
	var deleteSet map[digest.Digest]struct{}
	var blobCount int

	if opts.SweepOnly && loadedCandidates != nil {
		// Optimization: In sweep-only mode, skip full blob enumeration
		// We already have candidates from checkpoint, just filter against new markSet
		logger.Info("Mark phase (2/2: blob enumeration) - SKIPPED (filtering checkpoint candidates in-memory)")
		blobEnumStart := time.Now()

		deleteSet = make(map[digest.Digest]struct{})
		protectedCount := 0

		for dgst := range loadedCandidates {
			if _, isMarked := markSet[dgst]; !isMarked {
				// Still unmarked, safe to delete
				deleteSet[dgst] = struct{}{}
			} else {
				// Now marked (new push referenced it), protect it
				protectedCount++
			}
		}

		blobCount = len(loadedCandidates)
		stats.BlobEnumDuration = time.Since(blobEnumStart)
		stats.TotalMarkDuration = stats.MarkDuration + stats.BlobEnumDuration

		logger.Infof("Mark phase (2/2: blob enumeration) complete: filtered %d checkpoint candidates, %d eligible for deletion, %d protected by new references (duration=%v)",
			blobCount, len(deleteSet), protectedCount, stats.BlobEnumDuration)
	} else {
		// Full blob enumeration for mark-only or full GC mode
		logger.Info("Starting mark phase (2/2: blob enumeration)")
		blobService := registry.Blobs()
		deleteSet = make(map[digest.Digest]struct{})
		blobEnumStart := time.Now()
		lastBlobProgress := time.Now()
		blobCount = 0

		err = blobService.Enumerate(ctx, func(dgst digest.Digest) error {
			blobCount++

			// Progress reporting every 30s (always show, even in quiet mode)
			if time.Since(lastBlobProgress) >= opts.ProgressInterval {
				elapsed := time.Since(blobEnumStart)
				logger.Infof("Mark progress (2/2: blob enumeration): checked=%d blobs (elapsed=%v, rate=%.0f blobs/sec)",
					blobCount, elapsed, float64(blobCount)/elapsed.Seconds())
				lastBlobProgress = time.Now()
			}

			// Check for cancellation every 10k blobs
			if blobCount%10000 == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}

			if _, ok := markSet[dgst]; !ok {
				deleteSet[dgst] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error enumerating blobs: %v", err)
		}

		// Log blob enumeration completion
		stats.BlobEnumDuration = time.Since(blobEnumStart)
		stats.TotalMarkDuration = stats.MarkDuration + stats.BlobEnumDuration
		logger.Infof("Mark phase (2/2: blob enumeration) complete: total=%d blobs, candidates=%d duration=%v",
			blobCount, len(deleteSet), stats.BlobEnumDuration)
	}

	// If mark-only mode, save checkpoint and exit
	if opts.MarkOnly {
		// Convert deleteSet to string slice for JSON
		candidates := make([]string, 0, len(deleteSet))
		for dgst := range deleteSet {
			candidates = append(candidates, dgst.String())
		}

		checkpoint := CheckpointState{
			Version:            "1",
			Timestamp:          time.Now(),
			MarkPhaseComplete:  true,
			Stats:              *stats,
			DeletionCandidates: candidates,
		}

		if err := saveCheckpoint(opts.CheckpointDir, checkpoint); err != nil {
			return fmt.Errorf("failed to save checkpoint: %v", err)
		}

		logger.Infof("Mark phase complete: saved %d deletion candidates to %s",
			len(candidates), opts.CheckpointDir)
		return nil // Exit without sweep
	}

	// Sweep phase
	sweepStart := time.Now()
	lastProgress = time.Now() // Reset for sweep phase progress
	logger.Info("Starting sweep phase")

	vacuum := NewVacuum(ctx, storageDriver)
	if !opts.DryRun && len(manifestArr) > 0 {
		// Parallel manifest deletion using worker pool
		logger.Infof("Deleting %d manifests using %d workers", len(manifestArr), opts.MaxConcurrency)
		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(opts.MaxConcurrency)

		var manifestDeleteMu sync.Mutex
		manifestDeleteCount := 0

		for _, obj := range manifestArr {
			g.Go(func() error {
				// Check for context cancellation
				select {
				case <-groupCtx.Done():
					return groupCtx.Err()
				default:
				}

				err := vacuum.RemoveManifest(obj.Name, obj.Digest, obj.Tags)
				if err != nil {
					return fmt.Errorf("failed to delete manifest %s: %v", obj.Digest, err)
				}

				manifestDeleteMu.Lock()
				manifestDeleteCount++
				if manifestDeleteCount%100 == 0 && time.Since(lastProgress) >= opts.ProgressInterval {
					elapsed := time.Since(sweepStart)
					rate := float64(manifestDeleteCount) / elapsed.Seconds()
					logger.Infof("Sweep progress (manifests): deleted=%d/%d (elapsed=%v, rate=%.1f manifests/sec)",
						manifestDeleteCount, len(manifestArr), elapsed, rate)
					lastProgress = time.Now()
				}
				manifestDeleteMu.Unlock()

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
		stats.ManifestsDeleted = manifestDeleteCount
		logger.Infof("Manifest deletion complete: deleted=%d duration=%v", manifestDeleteCount, time.Since(sweepStart))
	}

	// deleteSet and blobService already populated above
	if !opts.Quiet {
		emit("\n%d blobs marked, %d blobs and %d manifests eligible for deletion", len(markSet), len(deleteSet), len(manifestArr))
	}

	// Parallel blob deletion using worker pool
	if !opts.DryRun && len(deleteSet) > 0 {
		logger.Infof("Deleting %d blobs using %d workers", len(deleteSet), opts.MaxConcurrency)

		// Convert deleteSet to slice for parallel processing
		deleteBlobs := make([]digest.Digest, 0, len(deleteSet))
		for dgst := range deleteSet {
			deleteBlobs = append(deleteBlobs, dgst)
		}

		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(opts.MaxConcurrency)

		var blobStatsMu sync.Mutex
		blobsDeleted := 0
		var totalBytes int64

		lastProgress = time.Now()

		for _, dgst := range deleteBlobs {
			g.Go(func() error {
				// Check for context cancellation
				select {
				case <-groupCtx.Done():
					return groupCtx.Err()
				default:
				}

				// Get blob size before deletion
				var blobSize int64
				blobPath := fmt.Sprintf("/docker/registry/v2/blobs/%s/%s/%s/data",
					dgst.Algorithm(), dgst.Hex()[0:2], dgst.Hex())
				if fi, err := storageDriver.Stat(groupCtx, blobPath); err == nil {
					blobSize = fi.Size()
				}

				err := vacuum.RemoveBlob(string(dgst))
				if err != nil {
					return fmt.Errorf("failed to delete blob %s: %v", dgst, err)
				}

				blobStatsMu.Lock()
				blobsDeleted++
				totalBytes += blobSize
				if blobsDeleted%1000 == 0 && time.Since(lastProgress) >= opts.ProgressInterval {
					elapsed := time.Since(sweepStart)
					rate := float64(blobsDeleted) / elapsed.Seconds()
					logger.Infof("Sweep progress (blobs): deleted=%d/%d (elapsed=%v, rate=%.1f blobs/sec)",
						blobsDeleted, len(deleteBlobs), elapsed, rate)
					lastProgress = time.Now()
				}
				blobStatsMu.Unlock()

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		stats.BlobsDeleted = blobsDeleted
		stats.BytesDeleted = totalBytes
		logger.Infof("Blob deletion complete: deleted=%d size=%s duration=%v",
			blobsDeleted, humanizeBytes(totalBytes), time.Since(sweepStart))
	} else if opts.DryRun {
		// Dry run mode - just count
		for dgst := range deleteSet {
			if !opts.Quiet {
				emit("blob eligible for deletion: %s", dgst)
			}
		}
	}

	for repo, dgsts := range deleteLayerSet {
		for _, dgst := range dgsts {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

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
			stats.LayerLinksDeleted++
		}
	}

	stats.SweepDuration = time.Since(sweepStart)
	logger.Infof("Sweep phase complete: manifests_deleted=%d blobs_deleted=%d space_freed=%s layer_links_deleted=%d duration=%v",
		stats.ManifestsDeleted, stats.BlobsDeleted, humanizeBytes(stats.BytesDeleted),
		stats.LayerLinksDeleted, stats.SweepDuration)

	// Clean up checkpoint file after successful sweep
	if opts.SweepOnly && opts.CheckpointDir != "" && err == nil {
		checkpointPath := filepath.Join(opts.CheckpointDir, "candidates.json")
		if removeErr := os.Remove(checkpointPath); removeErr != nil {
			logger.Warnf("Failed to remove checkpoint file %s: %v", checkpointPath, removeErr)
		} else {
			logger.Infof("Removed checkpoint file: %s", checkpointPath)
		}
	}

	return err
}

// unmarkReferencedManifest filters out manifest present in markSet
func unmarkReferencedManifest(manifestArr []ManifestDel, markSet map[digest.Digest]struct{}, quietOutput bool) []ManifestDel {
	filtered := make([]ManifestDel, 0)
	for _, obj := range manifestArr {
		if _, ok := markSet[obj.Digest]; !ok {
			if !quietOutput {
				emit("manifest eligible for deletion: repo=%s digest=%s", obj.Name, obj.Digest)
			}

			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// acquireLock creates a distributed lock file to prevent concurrent GC runs
func acquireLock(checkpointDir string, timeout time.Duration) error {
	lockPath := filepath.Join(checkpointDir, ".lock")

	// Check if lock exists and is still valid
	if data, err := os.ReadFile(lockPath); err == nil {
		var lock LockFile
		if err := json.Unmarshal(data, &lock); err == nil {
			// Check if lock is expired
			if time.Since(lock.Timestamp) < timeout {
				return fmt.Errorf("another GC instance is running (locked by %s at %v)", lock.Hostname, lock.Timestamp)
			}
		}
	}

	// Create lock
	hostname, _ := os.Hostname()
	lock := LockFile{
		Hostname:  hostname,
		PID:       os.Getpid(),
		Timestamp: time.Now(),
		Timeout:   timeout.String(),
	}

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock: %v", err)
	}

	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint dir: %v", err)
	}

	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %v", err)
	}

	return nil
}

// releaseLock removes the distributed lock file
func releaseLock(checkpointDir string) error {
	lockPath := filepath.Join(checkpointDir, ".lock")
	return os.Remove(lockPath)
}

// saveCheckpoint saves the current GC state to disk
func saveCheckpoint(checkpointDir string, state CheckpointState) error {
	if checkpointDir == "" {
		return nil
	}

	statePath := filepath.Join(checkpointDir, "candidates.json")
	tmpPath := statePath + ".tmp"

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %v", err)
	}

	// Atomic write: write to temp file, then rename
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint: %v", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		return fmt.Errorf("failed to rename checkpoint: %v", err)
	}

	return nil
}

// loadCheckpoint loads the saved GC state from disk
func loadCheckpoint(checkpointDir string) (*CheckpointState, error) {
	if checkpointDir == "" {
		return nil, nil
	}

	statePath := filepath.Join(checkpointDir, "candidates.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No checkpoint exists
		}
		return nil, fmt.Errorf("failed to read checkpoint: %v", err)
	}

	var state CheckpointState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %v", err)
	}

	// Validate checkpoint is not too old (7 days)
	if time.Since(state.Timestamp) > 7*24*time.Hour {
		return nil, fmt.Errorf("checkpoint is too old (%v), please delete and restart", time.Since(state.Timestamp))
	}

	if !state.MarkPhaseComplete {
		return nil, fmt.Errorf("checkpoint is incomplete, mark phase did not finish")
	}

	return &state, nil
}

// humanizeBytes converts bytes to human-readable format
func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
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
