package pruner

import (
	"bufio"
	"container/list"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const (
	confirmText = "Do you really want to permanently delete objects? [y/n]: "
)

type byName []string

func (ris byName) Len() int           { return len(ris) }
func (ris byName) Less(i, j int) bool { return ris[i] < ris[j] }
func (ris byName) Swap(i, j int)      { ris[i], ris[j] = ris[j], ris[i] }

// Pruner allows to collect information about registry from local filesystem
// and to prune no longer needed or invalid objects.
type Pruner struct {
	Config
	ctx context.Context
}

// NewPruner returns a new instance of pruner.
func NewPruner(c *Config, ctx context.Context) *Pruner {
	return &Pruner{*c, ctx}
}

// O is a helper method that Prints given message to command's output stream.
func (p *Pruner) O(format string, a ...interface{}) {
	fmt.Fprintf(p.Out, format, a...)
}

// LoadRegistryGraph scans a filesystem and returns registry's representation.
func (p *Pruner) LoadRegistryGraph(reg *Registry) (*storage.RegistryGraph, error) {
	sd, err := factory.Create(reg.config.Storage.Type(), reg.config.Storage.Parameters())
	if err != nil {
		return nil, err
	}
	if p.Verbose {
		fmt.Fprintf(p.Out, "Exploring storage \"%s\" [ver %s] ...\n", sd.Name(), driver.CurrentVersion)
	}
	vacuum := storage.NewVacuum(p.ctx, sd)
	rg, err := storage.LoadRegistryGraph(&vacuum)
	if err != nil {
		return nil, err
	}
	return rg, nil
}

// ProcessRegistryGraph prints what can be cleaned in given registry and prunes it if told.
func (p *Pruner) ProcessRegistryGraph(reg *Registry, rg *storage.RegistryGraph) error {
	dirty := p.PrintOrphanedObjects(rg)
	if !dirty {
		p.O("Registry is clean. Nothing to do.\n")
		return nil
	}
	if p.DryRun {
		return nil
	}

	if !p.Confirm || confirmPrune(p.In, p.Out) {
		return rg.PruneOrphanedObjects(p.RemoveEmpty)
	}
	p.O("Nothing changed.\n")

	return nil
}

// PrintOrphanedObjects prints information about all the objects that can be
// cleaned up.
func (p *Pruner) PrintOrphanedObjects(rg *storage.RegistryGraph) bool {
	what := "About to"
	suffix := "."
	if p.DryRun {
		what = "Would"
	}
	if p.Verbose {
		suffix = ":"
	}

	registryDirty := false
	// Collect additional blobs being unreferenced as a consequence of deleting
	// repository's object.
	unreferencedBlobs := list.New()

	if p.Verbose {
		p.O("Total repositories %d.\n", rg.TotalRepositories)
	}

	prealloc := len(rg.DirtyRepositories)
	if p.RemoveEmpty && len(rg.EmptyRepositories) > len(rg.DirtyRepositories) {
		prealloc = len(rg.EmptyRepositories)
	}
	repoList := make([]string, 0, prealloc)

	// Print empty repositories
	if p.RemoveEmpty {
		for name := range rg.EmptyRepositories {
			repoList = append(repoList, name)
		}
		sort.Sort(byName(repoList))
		if len(repoList) > 0 {
			if p.DryRun {
				p.O("%s remove %d empty repositories:\n", what, len(repoList))
			} else {
				p.O("%s to remove %d repositories:\n", what, len(repoList))
			}
			for _, name := range repoList {
				p.O("  %s\n", name)
				repo := rg.EmptyRepositories[name]
				for ldgst := range repo.Layers {
					unreferencedBlobs.PushBack(ldgst)
				}
				for _, mi := range repo.DanglingManifests {
					for _, sdgst := range mi.Signatures {
						unreferencedBlobs.PushBack(sdgst)
					}
				}
				for _, mi := range repo.UnlinkedManifests {
					for _, sdgst := range mi.Signatures {
						unreferencedBlobs.PushBack(sdgst)
					}
				}
			}
			registryDirty = true
		}
		repoList = repoList[0:0]
	}

	// Print dirty repositories
	for name := range rg.DirtyRepositories {
		if _, exists := rg.EmptyRepositories[name]; !p.RemoveEmpty || !exists {
			repoList = append(repoList, name)
		}
	}
	if len(repoList) > 0 {
		p.O("%s clean up %d dirty repositories:\n", what, len(repoList))
		sort.Sort(byName(repoList))
		for _, name := range repoList {
			p.O("  %s:\n", name)
			blobs := p.printOrphanedRepositoryObjects("    ", name, rg.DirtyRepositories[name])
			unreferencedBlobs.PushBackList(blobs)
		}
		registryDirty = true
	}

	// Unreference additional blobs
	var blobs storage.BlobRefCounter
	if unreferencedBlobs.Len() > 0 {
		// copy rg.Blobs map and decrease ref counters for additional unreferenced blobs
		blobs = make(map[digest.Digest]uint)
		for bdgst, rc := range rg.Blobs {
			blobs[bdgst] = rc
		}
		for e := unreferencedBlobs.Front(); e != nil; e = e.Next() {
			if val, exists := blobs[e.Value.(digest.Digest)]; exists {
				if val > 0 {
					blobs[e.Value.(digest.Digest)] = val - 1
				} else {
					context.GetLogger(p.ctx).Fatalf("There's a bug in reference counting!")
				}
			}
		}
	} else {
		blobs = rg.Blobs
	}

	// Print all orphaned blobs
	unreferencedBlobs = blobs.GetUnreferenced()
	if unreferencedBlobs.Len() > 0 {
		if p.DryRun {
			p.O("%s remove %d unreferenced blobs%s\n", what, unreferencedBlobs.Len(), suffix)
		} else {
			p.O("%s to remove %d unreferenced blobs%s\n", what, unreferencedBlobs.Len(), suffix)
		}
		if p.Verbose {
			for e := unreferencedBlobs.Front(); e != nil; e = e.Next() {
				p.O("  %s\n", e.Value.(digest.Digest).String())
			}
		}
		registryDirty = true
	}

	// Print void blobs
	if len(rg.VoidBlobs) > 0 {
		p.O("Would remove %d void blob directories%s\n", len(rg.VoidBlobs), suffix)
		if p.Verbose {
			for dgst := range rg.VoidBlobs {
				p.O("  %s\n", dgst.String())
			}
		}
	}

	return registryDirty
}

func (p *Pruner) printOrphanedRepositoryObjects(indent string, repoName string, rg *storage.RepositoryGraphInfo) *list.List {
	what := "About to"
	suffix := "."
	if p.DryRun {
		what = "Would"
	}
	if p.Verbose {
		suffix = ":"
	}

	unreferencedBlobs := rg.Layers.GetUnreferenced()

	if unreferencedBlobs.Len() > 0 {
		p.O("%s%s delete %d unreferenced layers%s\n", indent, what, unreferencedBlobs.Len(), suffix)
		if p.Verbose {
			for e := unreferencedBlobs.Front(); e != nil; e = e.Next() {
				p.O("%s  %s\n", indent, e.Value.(digest.Digest).String())
			}
		}
	}

	if len(rg.DanglingLayers) > 0 {
		p.O("%s%s delete %d dangling layers%s\n", indent, what, len(rg.DanglingLayers), suffix)
		if p.Verbose {
			for layerRef := range rg.DanglingLayers {
				p.O("%s  %s\n", indent, layerRef)
			}
		}
	}

	if len(rg.UnlinkedLayers) > 0 {
		p.O("%s%s clear %d unlinked layers%s\n", indent, what, len(rg.UnlinkedLayers), suffix)
		if p.Verbose {
			for layerRef := range rg.UnlinkedLayers {
				p.O("%s  %s\n", indent, layerRef)
			}
		}
	}

	if len(rg.DanglingManifests) > 0 {
		p.O("%s%s delete %d dangling manifest revisions%s\n", indent, what, len(rg.DanglingManifests), suffix)
		for mRef, mi := range rg.DanglingManifests {
			if p.Verbose {
				p.O("%s  %s\n", indent, mRef)
			}
			for _, sRef := range mi.Signatures {
				if p.Verbose {
					p.O("%s    with signature %s\n", indent, sRef)
				}
				unreferencedBlobs.PushBack(sRef)
			}
		}
	}

	if len(rg.UnlinkedManifests) > 0 {
		p.O("%s%s clear %d unlinked manifest revisions%s\n", indent, what, len(rg.UnlinkedManifests), suffix)
		for mRef, mi := range rg.UnlinkedManifests {
			if p.Verbose {
				p.O("%s  %s\n", indent, mRef)
			}
			for _, sRef := range mi.Signatures {
				if p.Verbose {
					p.O("%s    with signature %s\n", indent, sRef)
				}
				unreferencedBlobs.PushBack(sRef)
			}
		}
	}

	for _, mi := range rg.Manifests {
		if len(mi.DanglingSignatures) > 0 {
			revisionName := repoName + ":" + mi.Tag
			p.O("%s%s delete %s dangling signatures from %s manifest revision%s\n", indent, what, len(mi.DanglingSignatures), revisionName, suffix)

			if p.Verbose {
				for _, sRef := range mi.DanglingSignatures {
					p.O("%s  %s\n", indent, sRef)
				}
			}
		}
	}

	if len(rg.DanglingTags) > 0 {
		p.O("%s%s delete %d dangling tags from %s repository%s\n", indent, what, len(rg.DanglingTags), repoName, suffix)
		for tag, dgst := range rg.DanglingTags {
			p.O("%s  %s -> %s\n", indent, tag, dgst.String())
		}
	}

	if len(rg.InvalidTags) > 0 {
		p.O("%s%s delete %d invalid tags from %s repository%s\n", indent, what, len(rg.InvalidTags), repoName, suffix)
		for _, tag := range rg.InvalidTags {
			p.O("%s  %s\n", indent, tag)
		}
	}

	return unreferencedBlobs
}

func confirmPrune(in io.Reader, out io.Writer) bool {
	answer := ""

	for answer != "n" && answer != "y" {
		fmt.Fprintf(out, confirmText)
		answer = strings.ToLower(strings.TrimSpace(readInput(in, out)))
	}

	return answer == "y"
}

func readInput(in io.Reader, out io.Writer) string {
	reader := bufio.NewReader(in)
	line, _, err := reader.ReadLine()
	if err != nil {
		fmt.Fprintln(out, err.Error())
		os.Exit(1)
	}
	return string(line)
}
