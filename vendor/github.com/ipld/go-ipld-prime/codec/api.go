package codec

import (
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// The following two types define the two directions of transform that a codec can be expected to perform:
// from Node to serial stream (aka "encoding", sometimes also described as "marshalling"),
// and from serial stream to Node (via a NodeAssembler) (aka "decoding", sometimes also described as "unmarshalling").
//
// You'll find a couple of implementations matching this shape in subpackages of 'codec'.
// (These are the handful of encoders and decoders we ship as "batteries included".)
// Other encoder and decoder implementations can be found in other repositories/modules.
// It should also be easy to implement encodecs and decoders of your own!
//
// Encoder and Decoder functions can be used on their own, but are also often used via the `ipld/linking.LinkSystem` construction,
// which handles all the other related operations necessary for a content-addressed storage system at once.
//
// Encoder and Decoder functions can be registered in the multicodec table in the `ipld/multicodec` package
// if they're providing functionality that matches the expectations for a multicodec identifier.
// This table will be used by some common EncoderChooser and DecoderChooser implementations
// (namely, the ones in LinkSystems produced by the `linking/cid` package).
// It's not strictly necessary to register functions there, though; you can also just use them directly.
//
// There are furthermore several conventions that codec packages are recommended to follow, but are only conventions:
//
// Most codec packages should have a ReusableEncoder and ResuableDecoder type,
// which contain any working memory needed by the implementation, as well as any configuration options,
// and those types should have an Encode and Decode function respectively which match these function types.
// They may alternatively have EncoderConfig and DecoderConfig types, which have similar purpose,
// but aren't promising memory reuse if kept around.
//
// By convention, a codec package that expects to fulfill a multicodec contract will also have
// a package-scope exported function called Encode or Decode which also matches this interface,
// and is the equivalent of creating a zero-value ReusableEncoder or ReusableDecoder (aka, default config)
// and using its Encode or Decode methods.
// This package-scope function may also internally use a sync.Pool
// to keep some ReusableEncoder values on hand to avoid unnecesary allocations.
//
// Note that an EncoderConfig or DecoderConfig type that supports configuration options
// does not functionally expose those options when invoked by the multicodec system --
// multicodec indicator codes do not provide room for extended configuration info.
// Codecs that expose configuration options are doing so for library users to enjoy;
// it does not mean those non-default configurations will necessarly be available
// in all scenarios that use codecs indirectly.
// There is also no standard interface for such configurations: by nature,
// if they exist at all, they tend to vary per codec.
type (
	// Encoder defines the shape of a function which traverses a Node tree
	// and emits its data in a serialized form into an io.Writer.
	//
	// The dual of Encoder is a Decoder, which takes a NodeAssembler
	// and fills it with deserialized data consumed from an io.Reader.
	// Typically, Decoder and Encoder functions will be found in pairs,
	// and will be expected to be able to round-trip each other's data.
	//
	// Encoder functions can be used directly.
	// Encoder functions are also often used via a LinkSystem when working with content-addressed storage.
	// LinkSystem methods will helpfully handle the entire process of traversing a Node tree,
	// encoding this data, hashing it, streaming it to the writer, and committing it -- all as one step.
	//
	// An Encoder works with Nodes.
	// If you have a native golang structure, and want to serialize it using an Encoder,
	// you'll need to figure out how to transform that golang structure into an ipld.Node tree first.
	//
	// It may be useful to understand "multicodecs" when working with Encoders.
	// In IPLD, a system called "multicodecs" is typically used to describe encoding foramts.
	// A "multicodec indicator" is a number which describes an encoding;
	// the Link implementations used in IPLD (CIDs) store a multicodec indicator in the Link;
	// and in this library, a multicodec registry exists in the `codec` package,
	// and can be used to associate a multicodec indicator number with an Encoder function.
	// The default EncoderChooser in a LinkSystem will use this multicodec registry to select Encoder functions.
	// However, you can construct a LinkSystem that uses any EncoderChooser you want.
	// It is also possible to have and use Encoder functions that aren't registered as a multicodec at all...
	// we just recommend being cautious of this, because it may make your data less recognizable
	// when working with other systems that use multicodec indicators as part of their communication.
	Encoder func(datamodel.Node, io.Writer) error

	// Decoder defines the shape of a function which produces a Node tree
	// by reading serialized data from an io.Reader.
	// (Decoder doesn't itself return a Node directly, but rather takes a NodeAssembler as an argument,
	// because this allows the caller more control over the Node implementation,
	// as well as some control over allocations.)
	//
	// The dual of Decoder is an Encoder, which takes a Node and
	// emits its data in a serialized form into an io.Writer.
	// Typically, Decoder and Encoder functions will be found in pairs,
	// and will be expected to be able to round-trip each other's data.
	//
	// Decoder functions can be used directly.
	// Decoder functions are also often used via a LinkSystem when working with content-addressed storage.
	// LinkSystem methods will helpfully handle the entire process of opening block readers,
	// verifying the hash of the data stream, and applying a Decoder to build Nodes -- all as one step.
	//
	// A Decoder works with Nodes.
	// If you have a native golang structure, and want to populate it with data using a Decoder,
	// you'll need to either get a NodeAssembler which proxies data into that structure directly,
	// or assemble a Node as intermediate storage and copy the data to the native structure as a separate step.
	//
	// It may be useful to understand "multicodecs" when working with Decoders.
	// See the documentation on the Encoder function interface for more discussion of multicodecs,
	// the multicodec table, and how this is typically connected to linking.
	Decoder func(datamodel.NodeAssembler, io.Reader) error
)

// -------------------
//  Errors
//

type ErrBudgetExhausted struct{}

func (e ErrBudgetExhausted) Error() string {
	return "decoder resource budget exhausted (message too long or too complex)"
}

// ---------------------
//  Other valuable and reused constants
//

type MapSortMode uint8

const (
	MapSortMode_None MapSortMode = iota
	MapSortMode_Lexical
	MapSortMode_RFC7049
)
