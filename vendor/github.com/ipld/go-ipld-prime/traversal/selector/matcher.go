package selector

import (
	"fmt"
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/basicnode"
)

// Matcher marks a node to be included in the "result" set.
// (All nodes traversed by a selector are in the "covered" set (which is a.k.a.
// "the merkle proof"); the "result" set is a subset of the "covered" set.)
//
// In libraries using selectors, the "result" set is typically provided to
// some user-specified callback.
//
// A selector tree with only "explore*"-type selectors and no Matcher selectors
// is valid; it will just generate a "covered" set of nodes and no "result" set.
// TODO: From spec: implement conditions and labels
type Matcher struct {
	*Slice
}

// Slice limits a result node to a subset of the node.
// The returned node will be limited based on slicing the specified range of the
// node into a new node, or making use of the `AsLargeBytes` io.ReadSeeker to
// restrict response with a SectionReader.
type Slice struct {
	From int64
	To   int64
}

func (s Slice) Slice(n datamodel.Node) (datamodel.Node, error) {
	var from, to int64
	switch n.Kind() {
	case datamodel.Kind_String:
		str, err := n.AsString()
		if err != nil {
			return nil, err
		}
		to = s.To
		if len(str) < int(to) {
			to = int64(len(str))
		}
		from = s.From
		if len(str) < int(from) {
			from = int64(len(str))
		}
		return basicnode.NewString(str[from:to]), nil
	case datamodel.Kind_Bytes:
		if lbn, ok := n.(datamodel.LargeBytesNode); ok {
			rdr, err := lbn.AsLargeBytes()
			if err != nil {
				return nil, err
			}

			sr := io.NewSectionReader(readerat{rdr}, s.From, s.To-s.From)
			return basicnode.NewBytesFromReader(sr), nil
		}
		bytes, err := n.AsBytes()
		if err != nil {
			return nil, err
		}
		to = s.To
		if len(bytes) < int(to) {
			to = int64(len(bytes))
		}
		from = s.From
		if len(bytes) < int(from) {
			from = int64(len(bytes))
		}

		return basicnode.NewBytes(bytes[from:to]), nil
	default:
		return nil, fmt.Errorf("selector slice rejected on %s: subset match must be over string or bytes", n.Kind())
	}
}

// Interests are empty for a matcher (for now) because
// It is always just there to match, not explore further
func (s Matcher) Interests() []datamodel.PathSegment {
	return []datamodel.PathSegment{}
}

// Explore will return nil because a matcher is a terminal selector
func (s Matcher) Explore(n datamodel.Node, p datamodel.PathSegment) (Selector, error) {
	return nil, nil
}

// Decide is always true for a match cause it's in the result set
// Deprecated: use Match instead
func (s Matcher) Decide(n datamodel.Node) bool {
	return true
}

// Match is always true for a match cause it's in the result set
func (s Matcher) Match(node datamodel.Node) (datamodel.Node, error) {
	if s.Slice != nil {
		return s.Slice.Slice(node)
	}
	return node, nil
}

// ParseMatcher assembles a Selector
// from a matcher selector node
// TODO: Parse labels and conditions
func (pc ParseContext) ParseMatcher(n datamodel.Node) (Selector, error) {
	if n.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map")
	}

	// check if a slice is specified
	if subset, err := n.LookupByString("subset"); err == nil {
		if subset.Kind() != datamodel.Kind_Map {
			return nil, fmt.Errorf("selector spec parse rejected: subset body must be a map")
		}
		from, err := subset.LookupByString("[")
		if err != nil {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with a from '[' key")
		}
		fromN, err := from.AsInt()
		if err != nil {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with a 'from' key that is a number")
		}
		to, err := subset.LookupByString("]")
		if err != nil {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with a to ']' key")
		}
		toN, err := to.AsInt()
		if err != nil {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with a 'to' key that is a number")
		}
		if fromN > toN {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with a 'from' key that is less than or equal to the 'to' key")
		}
		if fromN < 0 || toN < 0 {
			return nil, fmt.Errorf("selector spec parse rejected: selector body must be a map with keys not less than 0")
		}
		return Matcher{&Slice{
			From: fromN,
			To:   toN,
		}}, nil
	}
	return Matcher{}, nil
}
