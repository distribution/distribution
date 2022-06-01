package selector

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// ExploreAll is similar to a `*` -- it traverses all elements of an array,
// or all entries in a map, and applies a next selector to the reached nodes.
type ExploreAll struct {
	next Selector // selector for element we're interested in
}

// Interests for ExploreAll is nil (meaning traverse everything)
func (s ExploreAll) Interests() []datamodel.PathSegment {
	return nil
}

// Explore returns the node's selector for all fields
func (s ExploreAll) Explore(n datamodel.Node, p datamodel.PathSegment) (Selector, error) {
	return s.next, nil
}

// Decide always returns false because this is not a matcher
func (s ExploreAll) Decide(n datamodel.Node) bool {
	return false
}

// Match always returns false because this is not a matcher
func (s ExploreAll) Match(node datamodel.Node) (datamodel.Node, error) {
	return nil, nil
}

// ParseExploreAll assembles a Selector from a ExploreAll selector node
func (pc ParseContext) ParseExploreAll(n datamodel.Node) (Selector, error) {
	if n.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map")
	}
	next, err := n.LookupByString(SelectorKey_Next)
	if err != nil {
		return nil, fmt.Errorf("selector spec parse rejected: next field must be present in ExploreAll selector")
	}
	selector, err := pc.ParseSelector(next)
	if err != nil {
		return nil, err
	}
	return ExploreAll{selector}, nil
}
