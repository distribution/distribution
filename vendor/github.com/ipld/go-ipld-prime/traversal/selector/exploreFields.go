package selector

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// ExploreFields traverses named fields in a map (or equivalently, struct, if
// traversing on typed/schema nodes) and applies a next selector to the
// reached nodes.
//
// Note that a concept of "ExplorePath" (e.g. "foo/bar/baz") can be represented
// as a set of three nexted ExploreFields selectors, each specifying one field.
// (For this reason, we don't have a special "ExplorePath" feature; use this.)
//
// ExploreFields also works for selecting specific elements out of a list;
// if a "field" is a base-10 int, it will be coerced and do the right thing.
// ExploreIndex or ExploreRange is more appropriate, however, and should be preferred.
type ExploreFields struct {
	selections map[string]Selector
	interests  []datamodel.PathSegment // keys of above; already boxed as that's the only way we consume them
}

// Interests for ExploreFields are the fields listed in the selector node
func (s ExploreFields) Interests() []datamodel.PathSegment {
	return s.interests
}

// Explore returns the selector for the given path if it is a field in
// the selector node or nil if not
func (s ExploreFields) Explore(n datamodel.Node, p datamodel.PathSegment) (Selector, error) {
	return s.selections[p.String()], nil
}

// Decide always returns false because this is not a matcher
func (s ExploreFields) Decide(n datamodel.Node) bool {
	return false
}

// Match always returns false because this is not a matcher
func (s ExploreFields) Match(node datamodel.Node) (datamodel.Node, error) {
	return nil, nil
}

// ParseExploreFields assembles a Selector
// from a ExploreFields selector node
func (pc ParseContext) ParseExploreFields(n datamodel.Node) (Selector, error) {
	if n.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map")
	}
	fields, err := n.LookupByString(SelectorKey_Fields)
	if err != nil {
		return nil, fmt.Errorf("selector spec parse rejected: fields in ExploreFields selector must be present")
	}
	if fields.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: fields in ExploreFields selector must be a map")
	}
	x := ExploreFields{
		make(map[string]Selector, fields.Length()),
		make([]datamodel.PathSegment, 0, fields.Length()),
	}
	for itr := fields.MapIterator(); !itr.Done(); {
		kn, v, err := itr.Next()
		if err != nil {
			return nil, fmt.Errorf("error during selector spec parse: %w", err)
		}

		kstr, _ := kn.AsString()
		x.interests = append(x.interests, datamodel.PathSegmentOfString(kstr))
		x.selections[kstr], err = pc.ParseSelector(v)
		if err != nil {
			return nil, err
		}
	}
	return x, nil
}
