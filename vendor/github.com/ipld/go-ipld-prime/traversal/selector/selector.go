package selector

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// Selector is a "compiled" and executable IPLD Selector.
// It can be put to work with functions like traversal.Walk,
// which will use the Selector's guidance to decide how to traverse an IPLD data graph.
// A user will not generally call any of the methods of Selector themselves, nor implement the interface;
// it is produced by "compile" functions in this package, and used by functions in the `traversal` package.
//
// A Selector is created by parsing an IPLD Data Model document that declares a Selector
// (this is accomplished with functions like CompileSelector).
// To make this even easier, there is a `parse` subpackage,
// which contains helper methods for parsing direction from a JSON Selector document to a compiled Selector value.
// Alternatively, there is a `builder` subpackage,
// which may be useful if you would rather create the Selector declaration programmatically in golang
// (however, we recommend using this sparingly, because part of what makes Selectors cool is their language-agnostic declarative nature).
//
// There is no way to go backwards from this "compiled" Selector type into the declarative IPLD data model information that produced it.
// That declaration information is discarded after compilation in order to limit the amount of memory held.
// Therefore, if you're building APIs about Selector composition, keep in mind that
// you'll probably want to approach this be composing the Data Model declaration documents,
// and you should *not* attempt to be composing this type, which is only for the "compiled" result.
type Selector interface {
	// Notes for you who implements a Selector:
	// this type holds the state describing what we will do at one step in a traversal.
	// The actual traversal stepping is applied *from the outside* (and this is implemented mostly in the `traversal` package;
	// this type just gives it instructions on how to step.
	// Each of the functions on this type should be pure; they can can read the Selector's fields, but should treat them as config, not as state -- the Selector should never mutate.
	//
	// The traversal process will ask things of a Selector in three phases,
	// and control flow will bounce back and forth between traversal logic and selector evaluation --
	// traversal owns the actual walking (and any data loading), and just briefly dips down into the Selector so it can answer questions:
	//   T1. Traversal starts at some Node with some Selector.
	//   S1. First, the traversal asks the Selector what its "interests" are.
	//        This lets the Selector hint to the traversal process what it should load,
	//        which can be important for performance if not all of the next data elements are in memory already.
	//        (This is applicable to ADLs which contain large sharded data, for example.)
	//        (The "interests" phase should be _fast_; more complicated checks, and anything that actually looks at the children, should wait until the "explore" phase;
	//        in fact, for this reason, the `Interests` function doesn't even get to look at the data at all yet.)
	//   T2. The traversal looks at the Node and its actual fields, and what the Selector just said are interesting,
	//        and between the two of them figures out what's actually here to act on.
	//        (Note that the Selector can say that certain paths are interesting, and that path can then not be there.)
	//   S2. Second, the code driving the traversal will ask us to "explore", **stepwise**.
	//        The "explore" step is applied **repeatedly**: once per pathSegment that identifies a child in the Node.
	//        (If `Interests()` returned a list, `Explore` will be called for each element in the list (as long as that pathSegment actually existed in the Node, of course);
	//        or if `Interest()` returned no guidance, `Explore` will be called for everything in the object.)
	//   S2.a.  The "explore" step returns a new Selector object, with instructions about how to continue the walk for the reached object and beneath.
	//            (Note that the "explore" step can also return `nil` here to say "actually, don't look any further",
	//            and it may do so even if the "interests" phase suggested there might be something to follow up on here.  (Remember "interests" had to be fast, and was a first pass only.))
	//   T2.a.  ***Recursion time!***
	//            The traversal now takes that pathSegment and that subsequent Selector produced by `Explore`,
	//            gets the child Node at that pathSegment, and recurses into traversing on that Node with that Selector!
	//            It is also possibly ***link load time***, right before recursing:
	//            if the child node is a Link, the traversal may choose to load it now,
	//            and then do the recursion on the loaded Node (instead of on the actual direct child Node, which was a Link) with the next Selector.
	//   T2.b.  When the recursion is done, the traversal goes on to repeat S2, with the next pathSegment,
	//            until it runs out of things to do.
	//   T3.  The traversal asks the Selector to "decide" if this current Node is one that is "matched or not.
	//        See the Selector specs for discussion on "matched" vs "reached"/"visited" nodes.
	//        (Long story short: the traversal probably fires off callbacks for "matched" nodes, aka if `Decide` says `true`.)
	//   S3.  The selector does so.
	//   T4.  The traversal for this node is done.
	//
	// Phase T3+S3 can also be T0+S0, which makes for a pre-order traversal instead of a post-order traversal.
	// The Selector doesn't know the difference.
	// (In particular, a Selector implementation absolutely may **not** assume `Decide` will be called before `Interests`, and may **not** hold onto a Node statefully, etc.)
	//
	// Note that it's not until phase T2.a that the traversal actually loads child Nodes.
	// This is interesting because it's *after* when the Selector is asked to `Explore` and yield a subsequent Selector to use on that upcoming Node.
	//
	// Can `Explore` and `Decide` do Link loading on their own?  Do they need to?
	// Right now, no, they can't.  (Sort of.)  They don't have access to a LinkLoader; the traversal would have to give them one.
	// This might be needed in the future, e.g. if the Selector has a Condition clause that requires looking deeper; so far, we don't have those features, so it hasn't been needed.
	// The "sort of" is for ADLs.  ADLs that work with large sharded data sometimes hold onto their own LinkLoader and apply it transparently.
	// In that case, of course, `Explore` and `Decide` can just interrogate the Node they've been given, and that may cause link loading.
	// (If that happens, we're currently assuming the ADL has a reasonable caching behavior.  It's very likely that the traversal will look up the same paths that Explore just looked up (assuming the Condition told exploration to continue).)
	//

	// Interests should return either a list of PathSegment we're likely interested in,
	// **or nil**, which indicates we're a high-cardinality or expression-based selection clause and thus we'll need all segments proposed to us.
	// Note that a non-nil zero length list of PathSegment is distinguished from nil: this would mean this selector is interested absolutely nothing.
	//
	// Traversal will call this before calling Explore, and use it to try to call Explore less often (or even avoid iterating on the data node at all).
	Interests() []datamodel.PathSegment

	// Explore is told about the node we're at, and the pathSegment inside it to consider,
	// and returns either nil, if we shouldn't explore that path any further,
	// or returns a Selector, which should then be used to explore the child at that path.
	//
	// Note that the node parameter is not the child, it's the node we're currently at.
	// (Often, this is sufficient information: consider ExploreFields,
	// which only even needs to regard the pathSegment, and not the node at all.)
	//
	// Remember that Explore does **not** iterate `node` itself; the visits to any children of `node` will be driven from the outside, by the traversal function.
	// (The Selector's job is just guiding that process by returning information.)
	// The architecture works this way so that a sufficiently clever traversal function could consider several reasons for exploring a node before deciding whether to do so.
	Explore(node datamodel.Node, child datamodel.PathSegment) (subsequent Selector, err error)

	// Decide returns true if the subject node is "matched".
	//
	// Only "Matcher" clauses actually implement this in a way that ever returns "true".
	// See the Selector specs for discussion on "matched" vs "reached"/"visited" nodes.
	Decide(node datamodel.Node) bool

	// Match is an extension to Decide allowing the matcher to `decide` a transformation of
	// the matched node. This is used for `Subset` match behavior. If the node is matched,
	// the first argument will be the matched node. If it is not matched, the first argument
	// will be null. If there is an error, the first argument will be null.
	Match(node datamodel.Node) (datamodel.Node, error)
}

// REVIEW: do ParsedParent and ParseContext need to be exported?  They're mostly used during the compilation process.

// ParsedParent is created whenever you are parsing a selector node that may have
// child selectors nodes that need to know it
type ParsedParent interface {
	Link(s Selector) bool
}

// ParseContext tracks the progress when parsing a selector
type ParseContext struct {
	parentStack []ParsedParent
}

// CompileSelector accepts a datamodel.Node which should contain data that declares a Selector.
// The data layout expected for this declaration is documented in https://datamodel.io/specs/selectors/ .
//
// If the Selector is compiled successfully, it is returned.
// Otherwise, if the given data Node doesn't match the expected shape for a Selector declaration,
// or there are any other problems compiling the selector
// (such as a recursion edge with no enclosing recursion declaration, etc),
// then nil and an error will be returned.
func CompileSelector(dmt datamodel.Node) (Selector, error) {
	return ParseContext{}.ParseSelector(dmt)
}

// ParseSelector is an alias for CompileSelector, and is deprecated.
// Prefer CompileSelector.
func ParseSelector(dmt datamodel.Node) (Selector, error) {
	return CompileSelector(dmt)
}

// ParseSelector creates a Selector from an IPLD Selector Node with the given context
func (pc ParseContext) ParseSelector(n datamodel.Node) (Selector, error) {
	if n.Kind() != datamodel.Kind_Map {
		return nil, fmt.Errorf("selector spec parse rejected: selector is a keyed union and thus must be a map")
	}
	if n.Length() != 1 {
		return nil, fmt.Errorf("selector spec parse rejected: selector is a keyed union and thus must be single-entry map")
	}
	kn, v, _ := n.MapIterator().Next()
	kstr, _ := kn.AsString()
	// Switch over the single key to determine which selector body comes next.
	//  (This switch is where the keyed union discriminators concretely happen.)
	switch kstr {
	case SelectorKey_ExploreFields:
		return pc.ParseExploreFields(v)
	case SelectorKey_ExploreAll:
		return pc.ParseExploreAll(v)
	case SelectorKey_ExploreIndex:
		return pc.ParseExploreIndex(v)
	case SelectorKey_ExploreRange:
		return pc.ParseExploreRange(v)
	case SelectorKey_ExploreUnion:
		return pc.ParseExploreUnion(v)
	case SelectorKey_ExploreRecursive:
		return pc.ParseExploreRecursive(v)
	case SelectorKey_ExploreRecursiveEdge:
		return pc.ParseExploreRecursiveEdge(v)
	case SelectorKey_ExploreInterpretAs:
		return pc.ParseExploreInterpretAs(v)
	case SelectorKey_Matcher:
		return pc.ParseMatcher(v)
	default:
		return nil, fmt.Errorf("selector spec parse rejected: %q is not a known member of the selector union", kstr)
	}
}

// PushParent puts a parent onto the stack of parents for a parse context
func (pc ParseContext) PushParent(parent ParsedParent) ParseContext {
	l := len(pc.parentStack)
	parents := make([]ParsedParent, 0, l+1)
	parents = append(parents, parent)
	parents = append(parents, pc.parentStack...)
	return ParseContext{parents}
}

// SegmentIterator iterates either a list or a map, generating PathSegments
// instead of indexes or keys
type SegmentIterator interface {
	Next() (pathSegment datamodel.PathSegment, value datamodel.Node, err error)
	Done() bool
}

// NewSegmentIterator generates a new iterator based on the node type
func NewSegmentIterator(n datamodel.Node) SegmentIterator {
	if n.Kind() == datamodel.Kind_List {
		return listSegmentIterator{n.ListIterator()}
	}
	return mapSegmentIterator{n.MapIterator()}
}

type listSegmentIterator struct {
	datamodel.ListIterator
}

func (lsi listSegmentIterator) Next() (pathSegment datamodel.PathSegment, value datamodel.Node, err error) {
	i, v, err := lsi.ListIterator.Next()
	return datamodel.PathSegmentOfInt(i), v, err
}

func (lsi listSegmentIterator) Done() bool {
	return lsi.ListIterator.Done()
}

type mapSegmentIterator struct {
	datamodel.MapIterator
}

func (msi mapSegmentIterator) Next() (pathSegment datamodel.PathSegment, value datamodel.Node, err error) {
	k, v, err := msi.MapIterator.Next()
	if err != nil {
		return datamodel.PathSegment{}, v, err
	}
	kstr, _ := k.AsString()
	return datamodel.PathSegmentOfString(kstr), v, err
}

func (msi mapSegmentIterator) Done() bool {
	return msi.MapIterator.Done()
}
