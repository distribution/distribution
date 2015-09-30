package storage

import (
	"container/list"
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema1"
)

// graph contains functions for collecting information about orphaned objects
// and dangling links.
// These functions will only reliably work on strongly consistent storage
// systems.
// https://en.wikipedia.org/wiki/Consistency_model

// Empty is a type of set values using no space.
type Empty struct{}

// ManifestRevisionReferences maps digests to corresponding manifest revision objects.
type ManifestRevisionReferences map[digest.Digest]*ManifestRevisionGraphInfo

// BlobRefSet is a set of digests.
type BlobRefSet map[digest.Digest]Empty

// Add adds digest to the set.
func (bs BlobRefSet) Add(ds ...digest.Digest) {
	for _, d := range ds {
		bs[d] = Empty{}
	}
}

// Has returns true if given digest is present in the set.
func (bs BlobRefSet) Has(d digest.Digest) bool {
	_, exists := bs[d]
	return exists
}

// BlobRefCounter maps digests to its reference counter.
type BlobRefCounter map[digest.Digest]uint

// Add adds new digest entries to a map with a reference counter 0.
func (brc BlobRefCounter) Add(ds ...digest.Digest) {
	for _, d := range ds {
		brc[d] = 0
	}
}

// Has returns true if given digest is contained in the map.
func (brc BlobRefCounter) Has(d digest.Digest) bool {
	_, exists := brc[d]
	return exists
}

// Reference increments reference counter for given digest and returns its new
// value if the digest is present. Returns 0 otherwise.
func (brc BlobRefCounter) Reference(d digest.Digest) uint {
	if val, exists := brc[d]; exists {
		brc[d] = val + 1
		return val + 1
	}
	return 0
}

// Unreference decrements reference counter for given digest and retuns its new
// value. Decrementing the counter below 0 causes panic. It does nothing for
// unpresent digests.
func (brc BlobRefCounter) Unreference(d digest.Digest) uint {
	if val, exists := brc[d]; exists {
		if val == 0 {
			log.Fatalf("Decreasing reference count of blob %s below zero", d.String())
		}
		brc[d] = val - 1
		return val - 1
	}
	return 0
}

// HasUnreferenced returns true if the any of contained digests isn't
// referenced (its reference counter is 0).
func (brc BlobRefCounter) HasUnreferenced() bool {
	for _, rc := range brc {
		if rc == 0 {
			return true
		}
	}
	return false
}

// GetUnreferenced returns a list of unreferenced digests.
func (brc BlobRefCounter) GetUnreferenced() *list.List {
	res := list.New()
	for ref, rc := range brc {
		if rc == 0 {
			res.PushBack(ref)
		}
	}
	return res
}

// ManifestRevisionGraphInfo stores information about a manifest revision.
type ManifestRevisionGraphInfo struct {
	Tag        string
	Signatures []digest.Digest
	// DanglingSignatures is an array of signatures pointing to deleted blobs.
	DanglingSignatures []digest.Digest
	// RevisionLinkPath is used to check whether a manifest is stored at its
	// canonical path.
	// TODO: use this to move the revision to correct place.
	RevisionLinkPath string
}

// IsMisplaced returns true if the manifest revision isn't stored at canonical path.
// Version 2.1.0 unintentionally linked revisions into  _layers.
func (mi *ManifestRevisionGraphInfo) IsMisplaced(repoName string, dgst digest.Digest) bool {
	pth, err := manifestRevisionLinkPath(repoName, dgst)
	if err != nil {
		log.Fatalln(err)
	}
	return mi.RevisionLinkPath != pth
}

// IsDirty returns true if the manifest revision needs a purge.
func (mi *ManifestRevisionGraphInfo) IsDirty() bool {
	return len(mi.DanglingSignatures) > 0
}

// RepositoryGraphInfo contains information about a repository.
type RepositoryGraphInfo struct {
	Layers BlobRefCounter
	// UnlinkedLayers refer to empty <hex-digest> directories in _layers of the
	// repository.
	// TODO: there's no need to count references
	UnlinkedLayers BlobRefCounter
	// DanglingLayers is a set of layer digests pointing to deleted blobs.
	DanglingLayers BlobRefSet
	// Manifests contains manifest revisions which are either clean or dirty.
	Manifests ManifestRevisionReferences
	// UnlinkedManifests contains deleted manifest revisions that can be
	// cleaned up.
	UnlinkedManifests ManifestRevisionReferences
	// DanglingManifests contains manifest revisions refering to deleted blobs.
	DanglingManifests ManifestRevisionReferences
	// DanglingTags contains names of tags that point to manifest revistions no
	// longer presents as keys. Values are digests of deleted manifest
	// revisions.
	DanglingTags map[string]digest.Digest
	// InvalidTags is an array of tag names where a revision reference couldn't
	// be obtained.
	InvalidTags []string
	// TODO: collect unfinished uploads
}

// IsEmpty returns true if the repository contains no clean or dirty manifests
// and thus can be removed.
func (rg *RepositoryGraphInfo) IsEmpty() bool {
	// TODO: check for uploads in progress
	return len(rg.Manifests) == 0
}

// IsDirty returns true if the repository can be purged.
func (rg *RepositoryGraphInfo) IsDirty() bool {
	if len(rg.UnlinkedManifests) > 0 ||
		len(rg.DanglingManifests) > 0 ||
		len(rg.DanglingLayers) > 0 ||
		len(rg.DanglingTags) > 0 ||
		len(rg.InvalidTags) > 0 ||
		len(rg.DanglingLayers) > 0 ||
		len(rg.UnlinkedLayers) > 0 ||
		rg.Layers.HasUnreferenced() {
		return true
	}
	for _, mi := range rg.Manifests {
		if mi.IsDirty() {
			return true
		}
	}
	// TODO: check for unfinished uploads
	return false
}

// RegistryGraph stores registry objects that can be removed or cleaned up.
type RegistryGraph struct {
	Vacuum *Vacuum
	// Reference counter is increased for each:
	//  1. layer
	//  2. clean or dirty manifest revision
	//  3. signature
	Blobs BlobRefCounter
	// VoidBlobs are names (digests) of empty directories (no longer containing
	// any data) under blob root.
	VoidBlobs BlobRefSet
	// DirtyRepositories maps names of dirty repositories to their info objects.
	DirtyRepositories map[string]*RepositoryGraphInfo
	// EmptyRepositories maps names of empty repositories to their info objects.
	EmptyRepositories map[string]*RepositoryGraphInfo
	TotalRepositories uint
}

// UnreferencedBlobs returns a list of digests of blobs in global blob store
// having no referents.
func (rg *RegistryGraph) UnreferencedBlobs() *list.List {
	ubl := list.New()
	for dgst, rc := range rg.Blobs {
		if rc == 0 {
			ubl.PushBack(dgst)
		}
	}
	return ubl
}

// LoadRegistryGraph walks a filesystem and collects all the information needed
// for a clean up in a new instance of RegistryGraph.
func LoadRegistryGraph(v *Vacuum) (*RegistryGraph, error) {
	ns, err := NewRegistry(v.ctx, v.driver, EnableDelete)
	if err != nil {
		return nil, err
	}
	reg := ns.(*registry)

	blobs, void, err := loadRegistryBlobs(v.ctx, reg.blobStore)
	if err != nil {
		return nil, err
	}
	rg := &RegistryGraph{
		Vacuum:            v,
		Blobs:             blobs,
		VoidBlobs:         void,
		DirtyRepositories: make(map[string]*RepositoryGraphInfo),
		EmptyRepositories: make(map[string]*RepositoryGraphInfo),
	}

	if err = rg.loadRepositories(reg); err != nil {
		return nil, err
	}

	return rg, nil
}

// PruneOrphanedObjects deletes orphaned blobs, dangling links and empty
// directories. If removeEmpty is true, delets also empty repositories.
func (rg *RegistryGraph) PruneOrphanedObjects(removeEmpty bool) error {
	if removeEmpty {
		for name, ri := range rg.EmptyRepositories {
			if err := rg.Vacuum.RemoveRepository(name); err != nil {
				return err
			}
			for lRef := range ri.Layers {
				rg.Blobs.Unreference(lRef)
			}
			for _, mi := range ri.DanglingManifests {
				for _, sdgst := range mi.Signatures {
					rg.Blobs.Unreference(sdgst)
				}
			}
			for _, mi := range ri.UnlinkedManifests {
				for _, sdgst := range mi.Signatures {
					rg.Blobs.Unreference(sdgst)
				}
			}
		}
	}
	for name := range rg.DirtyRepositories {
		if _, exists := rg.EmptyRepositories[name]; removeEmpty && exists {
			continue
		}
		if err := rg.pruneDirtyRepository(name); err != nil {
			return err
		}
	}
	if err := rg.pruneOrphanedBlobs(); err != nil {
		return err
	}
	if err := rg.pruneVoidBlobs(); err != nil {
		return err
	}
	return nil
}

func (rg *RegistryGraph) loadRepository(repoName string, repo *repository) (*RepositoryGraphInfo, error) {
	manServ, err := repo.Manifests(rg.Vacuum.ctx)
	if err != nil {
		return nil, err
	}
	manStore := manServ.(*manifestStore)
	sigServ := repo.Signatures()
	sigStore := sigServ.(*signatureStore)
	ri := &RepositoryGraphInfo{
		Layers:            make(BlobRefCounter),
		DanglingLayers:    make(BlobRefSet),
		UnlinkedLayers:    make(BlobRefCounter),
		Manifests:         make(ManifestRevisionReferences),
		DanglingManifests: make(ManifestRevisionReferences),
		UnlinkedManifests: make(ManifestRevisionReferences),
		DanglingTags:      make(map[string]digest.Digest),
	}

	err = rg.loadRepositoryLayers(repo.Blobs(rg.Vacuum.ctx), ri)
	if err != nil {
		return nil, fmt.Errorf("Failed to load layers of repository %s: %v", repoName, err)
	}
	layers := make([]string, 0, len(ri.Layers))
	for l := range ri.Layers {
		layers = append(layers, l.String())
	}
	layers = make([]string, 0, len(ri.DanglingLayers))
	for l := range ri.DanglingLayers {
		layers = append(layers, l.String())
	}

	manRefs, err := manStore.revisionStore.list()
	if err != nil {
		return nil, err
	}

	for _, ref := range manRefs {
		var manifest *schema1.SignedManifest
		mi := &ManifestRevisionGraphInfo{
			Signatures: make([]digest.Digest, 0, 2),
		}
		if ri.Layers.Has(ref) || ri.DanglingLayers.Has(ref) || ri.UnlinkedLayers.Has(ref) {
			mi.RevisionLinkPath, err = blobLinkPath(repoName, ref)
		} else {
			mi.RevisionLinkPath, err = manifestRevisionLinkPath(repoName, ref)
		}
		if err != nil {
			// should not happen
			return nil, fmt.Errorf("Failed to resolve revision link path of %s@%s", repoName, ref)
		}

		// Consider it an unlinked layer
		if mi.IsMisplaced(repoName, ref) && !rg.Blobs.Has(ref) {
			continue
		}

		if !rg.Blobs.Has(ref) {
			ri.DanglingManifests[ref] = mi
		} else {
			manifest, err = manStore.Get(ref)
			if err != nil {
				// manifests are read also from `_layers` directory which most
				// certainly contains pointers to binary blobs. Ignore errors when
				// trying to read them as manifests.
				isUnknown := strings.Contains(strings.ToLower(err.Error()), "unknown manifest")
				// TODO: why "manifest unknown" when the manifest actually can't be parsed?
				if mi.IsMisplaced(repoName, ref) && isUnknown {
					continue
				}
				if isUnknown {
					ri.UnlinkedManifests[ref] = mi
				} else {
					// FIXME: add to BadManifests list for optional disposal?
					log.Warnf("Failed to load manifest revision %q: %v", ref.String(), err)
					rg.Blobs.Reference(ref)
				}
			} else {
				mi.Tag = manifest.Tag
				ri.Manifests[ref] = mi
				rg.Blobs.Reference(ref)
			}
		}

		if manifest != nil {
			for _, layer := range manifest.FSLayers {
				if ri.Layers.Reference(layer.BlobSum) == 0 {
					// FIXME: make manifest invalid
					qualif := "unknown"
					if ri.UnlinkedLayers.Reference(layer.BlobSum) > 0 {
						qualif = "unlinked"
					} else if ri.DanglingLayers.Has(layer.BlobSum) {
						qualif = "dangling"
					}
					log.Warnf("Manifest %s:%s@%s refers to %s layer %s", repoName, manifest.Tag, ref, qualif, layer.BlobSum)
				}
			}
		}

		signatures, err := sigStore.list(ref)
		if err != nil {
			log.Warnf("Failed to list signatures of %s: %v", repoName+"@"+ref.String(), err)
		} else {
			for _, signRef := range signatures {
				if rg.Blobs.Reference(signRef) == 0 {
					mi.DanglingSignatures = append(mi.DanglingSignatures, signRef)
				} else {
					mi.Signatures = append(mi.Signatures, signRef)
				}
			}
		}

		// TODO: Validate signatures / allow them to be recomputed
	}

	if err := loadRepositoryTags(repoName, manStore.tagStore, ri); err != nil {
		return nil, err
	}
	return ri, nil
}

func (rg *RegistryGraph) loadRepositoryLayers(bs distribution.BlobStore, ri *RepositoryGraphInfo) error {
	lbs := bs.(*linkedBlobStore)
	refs, err := lbs.list()
	if err != nil {
		return err
	}
	for e := refs.Front(); e != nil; e = e.Next() {
		ref := e.Value.(digest.Digest)
		if !rg.Blobs.Has(ref) {
			ri.DanglingLayers.Add(ref)
		} else {
			repoName := lbs.repository.Name()
			pth, err := pathFor(layerLinkPathSpec{
				name:   repoName,
				digest: ref,
			})
			if err != nil {
				log.Errorf("Failed to resolve layer link path spec for %s@%s", repoName, ref)
				continue
			}
			ok, err := exists(lbs.ctx, lbs.driver, pth)
			if err != nil {
				log.Errorf("Failed to resolve layer link path spec for %s@%s", repoName, err)
				continue
			}
			if !ok {
				ri.UnlinkedLayers.Add(ref)
			} else {
				ri.Layers.Add(ref)
				rg.Blobs.Reference(ref)
			}
		}
	}
	return nil
}

func (rg *RegistryGraph) loadRepositories(reg *registry) error {
	rl, err := reg.listRepositories(rg.Vacuum.ctx)
	if err != nil {
		return err
	}
	for e := rl.Front(); e != nil; e = e.Next() {
		repoName := e.Value.(string)
		repoServ, err := reg.Repository(rg.Vacuum.ctx, repoName)
		if err != nil {
			// FIXME: these could be added to BadRepositories list for optional disposal
			log.Warnf("Failed to load repository %q: %v", repoName, err)
			continue
		}
		rg.TotalRepositories++
		repo := repoServ.(*repository)
		repoGraph, err := rg.loadRepository(repoName, repo)
		if err != nil {
			// FIXME: these could be added to BadRepositories list for optional disposal
			log.Warnf("Failed to load repository %q: %v", repoName, err)
			continue
		}
		if repoGraph.IsDirty() {
			rg.DirtyRepositories[repoName] = repoGraph
		}
		if repoGraph.IsEmpty() {
			rg.EmptyRepositories[repoName] = repoGraph
		}
	}
	return err
}

func (rg *RegistryGraph) pruneDirtyRepository(repoName string) error {
	ri := rg.DirtyRepositories[repoName]
	toRemove := ri.Layers.GetUnreferenced()
	for e := toRemove.Front(); e != nil; e = e.Next() {
		lRef := e.Value.(digest.Digest)
		err := rg.Vacuum.UnlinkLayer(repoName, lRef.String(), true)
		if err != nil {
			return err
		}
		rg.Blobs.Unreference(lRef)
	}
	for lRef := range ri.DanglingLayers {
		err := rg.Vacuum.UnlinkLayer(repoName, lRef.String(), true)
		if err != nil {
			return err
		}
	}
	for lRef := range ri.UnlinkedLayers {
		err := rg.Vacuum.UnlinkLayer(repoName, lRef.String(), true)
		if err != nil {
			return err
		}
	}
	unlinkManifests := func(mrs ManifestRevisionReferences) error {
		for mRef, mi := range mrs {
			err := rg.Vacuum.UnlinkManifestRevision(repoName, mRef.String())
			if err != nil {
				return err
			}
			for _, sdgst := range mi.Signatures {
				rg.Blobs.Unreference(sdgst)
			}
		}
		return nil
	}
	if err := unlinkManifests(ri.DanglingManifests); err != nil {
		return err
	}
	if err := unlinkManifests(ri.UnlinkedManifests); err != nil {
		return err
	}

	for mRef, mi := range ri.Manifests {
		for _, sdgst := range mi.DanglingSignatures {
			rg.Vacuum.UnlinkSignature(repoName, mRef.String(), sdgst.String(), true)
		}
	}

	// TODO: allow to restore old references from index
	for tag := range ri.DanglingTags {
		rg.Vacuum.DeleteTag(repoName, tag)
	}

	// TODO: allow to restore old references from index
	for _, tag := range ri.InvalidTags {
		rg.Vacuum.DeleteTag(repoName, tag)
	}

	return nil
}

func (rg *RegistryGraph) pruneOrphanedBlobs() error {
	toRemove := rg.Blobs.GetUnreferenced()
	for e := toRemove.Front(); e != nil; e = e.Next() {
		err := rg.Vacuum.RemoveBlob(e.Value.(digest.Digest).String(), true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rg *RegistryGraph) pruneVoidBlobs() error {
	for dgst := range rg.VoidBlobs {
		err := rg.Vacuum.RemoveBlob(dgst.String(), true)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadRegistryBlobs(ctx context.Context, bs *blobStore) (BlobRefCounter, BlobRefSet, error) {
	bl, err := bs.list(ctx)
	if err != nil {
		return nil, nil, err
	}
	brc := make(BlobRefCounter)
	brs := make(BlobRefSet)
	for e := bl.Front(); e != nil; e = e.Next() {
		dgst := e.Value.(digest.Digest)
		path, err := pathFor(blobDataPathSpec{digest: dgst})
		if err != nil {
			log.Errorf("Failed to resolve blob data path spec for %s", dgst.String())
			continue
		}
		ok, err := exists(ctx, bs.driver, path)
		if err != nil {
			log.Errorf("Failed to check blob data path %s for existence", dgst.String())
			continue
		}
		if ok {
			brc.Add(dgst)
		} else {
			brs.Add(dgst)
		}
	}
	return brc, brs, nil
}

// loadRepositoryTags doesn't increment layer nor blob reference counters.
func loadRepositoryTags(repoName string, ts *tagStore, rg *RepositoryGraphInfo) error {
	tags, err := ts.tags()
	if err != nil {
		return err
	}
	for _, tag := range tags {
		dgst, err := ts.resolve(tag)
		if err != nil {
			log.Warnf("Couldn't resolve tag %s:%s: %v", repoName, tag, err)
			rg.InvalidTags = append(rg.InvalidTags, tag)
			continue
		}
		if _, ok := rg.Manifests[dgst]; !ok {
			rg.DanglingTags[tag] = dgst
		}
	}
	// TODO: parse tag index and allow to prune it
	return nil
}
