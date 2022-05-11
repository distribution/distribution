package linking

import (
	"context"
	"hash"
	"io"

	"github.com/ipld/go-ipld-prime/codec"
	"github.com/ipld/go-ipld-prime/datamodel"
)

// LinkSystem is a struct that composes all the individual functions
// needed to load and store content addressed data using IPLD --
// encoding functions, hashing functions, and storage connections --
// and then offers the operations a user wants -- Store and Load -- as methods.
//
// Typically, the functions which are fields of LinkSystem are not used
// directly by users (except to set them, when creating the LinkSystem),
// and it's the higher level operations such as Store and Load that user code then calls.
//
// The most typical way to get a LinkSystem is from the linking/cid package,
// which has a factory function called DefaultLinkSystem.
// The LinkSystem returned by that function will be based on CIDs,
// and use the multicodec registry and multihash registry to select encodings and hashing mechanisms.
// The BlockWriteOpener and BlockReadOpener must still be provided by the user;
// otherwise, only the ComputeLink method will work.
//
// Some implementations of BlockWriteOpener and BlockReadOpener may be
// found in the storage package.  Applications are also free to write their own.
// Custom wrapping of BlockWriteOpener and BlockReadOpener are also common,
// and may be reasonable if one wants to build application features that are block-aware.
type LinkSystem struct {
	EncoderChooser     func(datamodel.LinkPrototype) (codec.Encoder, error)
	DecoderChooser     func(datamodel.Link) (codec.Decoder, error)
	HasherChooser      func(datamodel.LinkPrototype) (hash.Hash, error)
	StorageWriteOpener BlockWriteOpener
	StorageReadOpener  BlockReadOpener
	TrustedStorage     bool
	NodeReifier        NodeReifier
	KnownReifiers      map[string]NodeReifier
}

// The following three types are the key functionality we need from a "blockstore".
//
// Some libraries might provide a "blockstore" object that has these as methods;
// it may also have more methods (like enumeration features, GC features, etc),
// but IPLD doesn't generally concern itself with those.
// We just need these key things, so we can "put" and "get".
//
// The functions are a tad more complicated than "put" and "get" so that they have good mechanical sympathy.
// In particular, the writing/"put" side is broken into two phases, so that the abstraction
// makes it easy to begin to write data before the hash that will identify it is fully computed.
type (
	// BlockReadOpener defines the shape of a function used to
	// open a reader for a block of data.
	//
	// In a content-addressed system, the Link parameter should be only
	// determiner of what block body is returned.
	//
	// The LinkContext may be zero, or may be used to carry extra information:
	// it may be used to carry info which hints at different storage pools;
	// it may be used to carry authentication data; etc.
	// (Any such behaviors are something that a BlockReadOpener implementation
	// will needs to document at a higher detail level than this interface specifies.
	// In this interface, we can only note that it is possible to pass such information opaquely
	// via the LinkContext or by attachments to the general-purpose Context it contains.)
	// The LinkContext should not have effect on the block body returned, however;
	// at most should only affect data availability
	// (e.g. whether any block body is returned, versus an error).
	//
	// Reads are cancellable by cancelling the LinkContext.Context.
	//
	// Other parts of the IPLD library suite (such as the traversal package, and all its functions)
	// will typically take a Context as a parameter or piece of config from the caller,
	// and will pass that down through the LinkContext, meaning this can be used to
	// carry information as well as cancellation control all the way through the system.
	//
	// BlockReadOpener is typically not used directly, but is instead
	// composed in a LinkSystem and used via the methods of LinkSystem.
	// LinkSystem methods will helpfully handle the entire process of opening block readers,
	// verifying the hash of the data stream, and applying a Decoder to build Nodes -- all as one step.
	//
	// BlockReadOpener implementations are not required to validate that
	// the contents which will be streamed out of the reader actually match
	// and hash in the Link parameter before returning.
	// (This is something that the LinkSystem composition will handle if you're using it.)
	//
	// BlockReadOpener can also be created out of storage.ReadableStorage and attached to a LinkSystem
	// via the LinkSystem.SetReadStorage method.
	//
	// Users of a BlockReadOpener function should also check the io.Reader
	// for matching the io.Closer interface, and use the Close function as appropriate if present.
	BlockReadOpener func(LinkContext, datamodel.Link) (io.Reader, error)

	// BlockWriteOpener defines the shape of a function used to open a writer
	// into which data can be streamed, and which will eventually be "commited".
	// Committing is done using the BlockWriteCommitter returned by using the BlockWriteOpener,
	// and finishes the write along with requiring stating the Link which should identify this data for future reading.
	//
	// The LinkContext may be zero, or may be used to carry extra information:
	// it may be used to carry info which hints at different storage pools;
	// it may be used to carry authentication data; etc.
	//
	// Writes are cancellable by cancelling the LinkContext.Context.
	//
	// Other parts of the IPLD library suite (such as the traversal package, and all its functions)
	// will typically take a Context as a parameter or piece of config from the caller,
	// and will pass that down through the LinkContext, meaning this can be used to
	// carry information as well as cancellation control all the way through the system.
	//
	// BlockWriteOpener is typically not used directly, but is instead
	// composed in a LinkSystem and used via the methods of LinkSystem.
	// LinkSystem methods will helpfully handle the entire process of traversing a Node tree,
	// encoding this data, hashing it, streaming it to the writer, and committing it -- all as one step.
	//
	// BlockWriteOpener implementations are expected to start writing their content immediately,
	// and later, the returned BlockWriteCommitter should also be able to expect that
	// the Link which it is given is a reasonable hash of the content.
	// (To give an example of how this might be efficiently implemented:
	// One might imagine that if implementing a disk storage mechanism,
	// the io.Writer returned from a BlockWriteOpener will be writing a new tempfile,
	// and when the BlockWriteCommiter is called, it will flush the writes
	// and then use a rename operation to place the tempfile in a permanent path based the Link.)
	//
	// BlockWriteOpener can also be created out of storage.WritableStorage and attached to a LinkSystem
	// via the LinkSystem.SetWriteStorage method.
	BlockWriteOpener func(LinkContext) (io.Writer, BlockWriteCommitter, error)

	// BlockWriteCommitter defines the shape of a function which, together
	// with BlockWriteOpener, handles the writing and "committing" of a write
	// to a content-addressable storage system.
	//
	// BlockWriteCommitter is a function which is will be called at the end of a write process.
	// It should flush any buffers and close the io.Writer which was
	// made available earlier from the BlockWriteOpener call that also returned this BlockWriteCommitter.
	//
	// BlockWriteCommitter takes a Link parameter.
	// This Link is expected to be a reasonable hash of the content,
	// so that the BlockWriteCommitter can use this to commit the data to storage
	// in a content-addressable fashion.
	// See the documentation of BlockWriteOpener for more description of this
	// and an example of how this is likely to be reduced to practice.
	BlockWriteCommitter func(datamodel.Link) error

	// NodeReifier defines the shape of a function that given a node with no schema
	// or a basic schema, constructs Advanced Data Layout node
	//
	// The LinkSystem itself is passed to the NodeReifier along with a link context
	// because Node interface methods on an ADL may actually traverse links to other
	// pieces of context addressed data that need to be loaded with the Link system
	//
	// A NodeReifier return one of three things:
	// - original node, no error = no reification occurred, just use original node
	// - reified node, no error = the simple node was converted to an ADL
	// - nil, error = the simple node should have been converted to an ADL but something
	// went wrong when we tried to do so
	//
	NodeReifier func(LinkContext, datamodel.Node, *LinkSystem) (datamodel.Node, error)
)

// LinkContext is a structure carrying ancilary information that may be used
// while loading or storing data -- see its usage in BlockReadOpener, BlockWriteOpener,
// and in the methods on LinkSystem which handle loading and storing data.
//
// A zero value for LinkContext is generally acceptable in any functions that use it.
// In this case, any operations that need a context.Context will quietly use Context.Background
// (thus being uncancellable) and simply have no additional information to work with.
type LinkContext struct {
	// Ctx is the familiar golang Context pattern.
	// Use this for cancellation, or attaching additional info
	// (for example, perhaps to pass auth tokens through to the storage functions).
	Ctx context.Context

	// Path where the link was encountered.  May be zero.
	//
	// Functions in the traversal package will set this automatically.
	LinkPath datamodel.Path

	// When traversing data or encoding: the Node containing the link --
	// it may have additional type info, etc, that can be accessed.
	// When building / decoding: not present.
	//
	// Functions in the traversal package will set this automatically.
	LinkNode datamodel.Node

	// When building data or decoding: the NodeAssembler that will be receiving the link --
	// it may have additional type info, etc, that can be accessed.
	// When traversing / encoding: not present.
	//
	// Functions in the traversal package will set this automatically.
	LinkNodeAssembler datamodel.NodeAssembler

	// Parent of the LinkNode.  May be zero.
	//
	// Functions in the traversal package will set this automatically.
	ParentNode datamodel.Node

	// REVIEW: ParentNode in LinkContext -- so far, this has only ever been hypothetically useful.  Keep or drop?
}
