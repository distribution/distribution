package selector

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// ExploreRecursiveEdge is a special sentinel value which is used to mark
// the end of a sequence started by an ExploreRecursive selector: the recursion
// goes back to the initial state of the earlier ExploreRecursive selector,
// and proceeds again (with a decremented maxDepth value).
//
// An ExploreRecursive selector that doesn't contain an ExploreRecursiveEdge
// is nonsensical.  Containing more than one ExploreRecursiveEdge is valid.
// An ExploreRecursiveEdge without an enclosing ExploreRecursive is an error.
type ExploreRecursiveEdge struct{}

// Interests should almost never get called for an ExploreRecursiveEdge selector
func (s ExploreRecursiveEdge) Interests() []datamodel.PathSegment {
	return []datamodel.PathSegment{}
}

// Explore should ultimately never get called for an ExploreRecursiveEdge selector
func (s ExploreRecursiveEdge) Explore(n datamodel.Node, p datamodel.PathSegment) (Selector, error) {
	panic("Traversed Explore Recursive Edge Node With No Parent")
}

// Decide should almost never get called for an ExploreRecursiveEdge selector
func (s ExploreRecursiveEdge) Decide(n datamodel.Node) bool {
	return false
}

// Match always returns false because this is not a matcher
func (s ExploreRecursiveEdge) Match(node datamodel.Node) (datamodel.Node, error) {
	return nil, nil
}

// ParseExploreRecursiveEdge assembles a Selector
// from a exploreRecursiveEdge selector node
func (pc ParseContext) ParseExploreRecursiveEdge(n datamodel.Node) (Selector, error) {
	if n.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map")
	}
	s := ExploreRecursiveEdge{}
	for _, parent := range pc.parentStack {
		if parent.Link(s) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("selector spec parse rejected: ExploreRecursiveEdge must be beneath ExploreRecursive")
}
