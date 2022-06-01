package traversal

import (
	"context"
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/linking"
)

// This file defines interfaces for things users provide,
//  plus a few of the parameters they'll need to receieve.
//--------------------------------------------------------

// VisitFn is a read-only visitor.
type VisitFn func(Progress, datamodel.Node) error

// TransformFn is like a visitor that can also return a new Node to replace the visited one.
type TransformFn func(Progress, datamodel.Node) (datamodel.Node, error)

// AdvVisitFn is like VisitFn, but for use with AdvTraversal: it gets additional arguments describing *why* this node is visited.
type AdvVisitFn func(Progress, datamodel.Node, VisitReason) error

// VisitReason provides additional information to traversals using AdvVisitFn.
type VisitReason byte

const (
	VisitReason_SelectionMatch     VisitReason = 'm' // Tells AdvVisitFn that this node was explicitly selected.  (This is the set of nodes that VisitFn is called for.)
	VisitReason_SelectionParent    VisitReason = 'p' // Tells AdvVisitFn that this node is a parent of one that will be explicitly selected.  (These calls only happen if the feature is enabled -- enabling parent detection requires a different algorithm and adds some overhead.)
	VisitReason_SelectionCandidate VisitReason = 'x' // Tells AdvVisitFn that this node was visited while searching for selection matches.  It is not necessarily implied that any explicit match will be a child of this node; only that we had to consider it.  (Merkle-proofs generally need to include any node in this group.)
)

type Progress struct {
	Cfg       *Config
	Path      datamodel.Path // Path is how we reached the current point in the traversal.
	LastBlock struct {       // LastBlock stores the Path and Link of the last block edge we had to load.  (It will always be zero in traversals with no linkloader.)
		Path datamodel.Path
		Link datamodel.Link
	}
	PastStartAtPath bool                        // Indicates whether the traversal has progressed passed the StartAtPath in the config -- use to avoid path checks when inside a sub portion of a DAG that is entirely inside the "not-skipped" portion of a traversal
	Budget          *Budget                     // If present, tracks "budgets" for how many more steps we're willing to take before we should halt.
	SeenLinks       map[datamodel.Link]struct{} // Set used to remember which links have been visited before, if Cfg.LinkVisitOnlyOnce is true.
}

type Config struct {
	Ctx                            context.Context                // Context carried through a traversal.  Optional; use it if you need cancellation.
	LinkSystem                     linking.LinkSystem             // LinkSystem used for automatic link loading, and also any storing if mutation features (e.g. traversal.Transform) are used.
	LinkTargetNodePrototypeChooser LinkTargetNodePrototypeChooser // Chooser for Node implementations to produce during automatic link traversal.
	LinkVisitOnlyOnce              bool                           // By default, we visit across links wherever we see them again, even if we've visited them before, because the reason for visiting might be different than it was before since we got to it via a different path.  If set to true, track links we've seen before in Progress.SeenLinks and do not visit them again.  Note that sufficiently complex selectors may require valid revisiting of some links, so setting this to true can change behavior noticably and should be done with care.
	StartAtPath                    datamodel.Path                 // If set, causes a traversal to skip forward until passing this path, and only then begins calling visit functions.  Block loads will also be skipped wherever possible.
}

type Budget struct {
	// Fields below are described as "monotonically-decrementing", because that's what the traversal library will do with them,
	// but they are user-accessable and can be reset to higher numbers again by code in the visitor callbacks.  This is not recommended (why?), but possible.

	// If you set any budgets (by having a non-nil Progress.Budget field), you must set some value for all of them.
	// Traversal halts when _any_ of the budgets reaches zero.
	// The max value of an int (math.MaxInt64) is acceptable for any budget you don't care about.

	NodeBudget int64 // A monotonically-decrementing "budget" for how many more nodes we're willing to visit before halting.
	LinkBudget int64 // A monotonically-decrementing "budget" for how many more links we're willing to load before halting.  (This is not aware of any caching; it's purely in terms of links encountered and traversed.)
}

// LinkTargetNodePrototypeChooser is a function that returns a NodePrototype based on
// the information in a Link and/or its LinkContext.
//
// A LinkTargetNodePrototypeChooser can be used in a traversal.Config to be clear about
// what kind of Node implementation to use when loading a Link.
// In a simple example, it could constantly return a `basicnode.Prototype.Any`.
// In a more complex example, a program using `bind` over native Go types
// could decide what kind of native type is expected, and return a
// `bind.NodeBuilder` for that specific concrete native type.
type LinkTargetNodePrototypeChooser func(datamodel.Link, linking.LinkContext) (datamodel.NodePrototype, error)

// SkipMe is a signalling "error" which can be used to tell traverse to skip some data.
//
// SkipMe can be returned by the Config.LinkLoader to skip entire blocks without aborting the walk.
// (This can be useful if you know you don't have data on hand,
// but want to continue the walk in other areas anyway;
// or, if you're doing a way where you know that it's valid to memoize seen
// areas based on Link alone.)
type SkipMe struct{}

func (SkipMe) Error() string {
	return "skip"
}

type ErrBudgetExceeded struct {
	BudgetKind string // "node"|"link"
	Path       datamodel.Path
	Link       datamodel.Link // only present if BudgetKind=="link"
}

func (e *ErrBudgetExceeded) Error() string {
	msg := fmt.Sprintf("traversal budget exceeded: budget for %ss reached zero while on path %q", e.BudgetKind, e.Path)
	if e.Link != nil {
		msg += fmt.Sprintf(" (link: %q)", e.Link)
	}
	return msg
}
