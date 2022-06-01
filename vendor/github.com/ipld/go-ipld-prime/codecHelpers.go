package ipld

import (
	"bytes"
	"io"
	"reflect"

	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
)

// Encode serializes the given Node using the given Encoder function,
// returning the serialized data or an error.
//
// The exact result data will depend the node content and on the encoder function,
// but for example, using a json codec on a node with kind map will produce
// a result starting in `{`, etc.
//
// Encode will automatically switch to encoding the representation form of the Node,
// if it discovers the Node matches the schema.TypedNode interface.
// This is probably what you want, in most cases;
// if this is not desired, you can use the underlaying functions directly
// (just look at the source of this function for an example of how!).
//
// If you would like this operation, but applied directly to a golang type instead of a Node,
// look to the Marshal function.
func Encode(n Node, encFn Encoder) ([]byte, error) {
	var buf bytes.Buffer
	err := EncodeStreaming(&buf, n, encFn)
	return buf.Bytes(), err
}

// EncodeStreaming is like Encode, but emits output to an io.Writer.
func EncodeStreaming(wr io.Writer, n Node, encFn Encoder) error {
	if tn, ok := n.(schema.TypedNode); ok {
		n = tn.Representation()
	}
	return encFn(n, wr)
}

// Decode parses the given bytes into a Node using the given Decoder function,
// returning a new Node or an error.
//
// The new Node that is returned will be the implementation from the node/basicnode package.
// This implementation of Node will work for storing any kind of data,
// but note that because it is general, it is also not necessarily optimized.
// If you want more control over what kind of Node implementation (and thus memory layout) is used,
// or want to use features like IPLD Schemas (which can be engaged by using a schema.TypedPrototype),
// then look to the DecodeUsingPrototype family of functions,
// which accept more parameters in order to give you that kind of control.
//
// If you would like this operation, but applied directly to a golang type instead of a Node,
// look to the Unmarshal function.
func Decode(b []byte, decFn Decoder) (Node, error) {
	return DecodeUsingPrototype(b, decFn, basicnode.Prototype.Any)
}

// DecodeStreaming is like Decode, but works on an io.Reader for input.
func DecodeStreaming(r io.Reader, decFn Decoder) (Node, error) {
	return DecodeStreamingUsingPrototype(r, decFn, basicnode.Prototype.Any)
}

// DecodeUsingPrototype is like Decode, but with a NodePrototype parameter,
// which gives you control over the Node type you'll receive,
// and thus control over the memory layout, and ability to use advanced features like schemas.
// (Decode is simply this function, but hardcoded to use basicnode.Prototype.Any.)
//
// DecodeUsingPrototype internally creates a NodeBuilder, and thows it away when done.
// If building a high performance system, and creating data of the same shape repeatedly,
// you may wish to use NodeBuilder directly, so that you can control and avoid these allocations.
//
// For symmetry with the behavior of Encode, DecodeUsingPrototype will automatically
// switch to using the representation form of the node for decoding
// if it discovers the NodePrototype matches the schema.TypedPrototype interface.
// This is probably what you want, in most cases;
// if this is not desired, you can use the underlaying functions directly
// (just look at the source of this function for an example of how!).
func DecodeUsingPrototype(b []byte, decFn Decoder, np NodePrototype) (Node, error) {
	return DecodeStreamingUsingPrototype(bytes.NewReader(b), decFn, np)
}

// DecodeStreamingUsingPrototype is like DecodeUsingPrototype, but works on an io.Reader for input.
func DecodeStreamingUsingPrototype(r io.Reader, decFn Decoder, np NodePrototype) (Node, error) {
	if tnp, ok := np.(schema.TypedPrototype); ok {
		np = tnp.Representation()
	}
	nb := np.NewBuilder()
	if err := decFn(nb, r); err != nil {
		return nil, err
	}
	return nb.Build(), nil
}

// Marshal accepts a pointer to a Go value and an IPLD schema type,
// and encodes the representation form of that data (which may be configured with the schema!)
// using the given Encoder function.
//
// Marshal uses the node/bindnode subsystem.
// See the documentation in that package for more details about its workings.
// Please note that this subsystem is relatively experimental at this time.
//
// The schema.Type parameter is optional, and can be nil.
// If given, it controls what kind of schema.Type (and what kind of representation strategy!)
// to use when processing the data.
// If absent, a default schema.Type will be inferred based on the golang type
// (so, a struct in go will be inferred to have a schema with a similar struct, and the default representation strategy (e.g. map), etc).
// Note that not all features of IPLD Schemas can be inferred from golang types alone.
// For example, to use union types, the schema parameter will be required.
// Similarly, to use most kinds of non-default representation strategy, the schema parameter is needed in order to convey that intention.
func Marshal(encFn Encoder, bind interface{}, typ schema.Type) ([]byte, error) {
	n := bindnode.Wrap(bind, typ)
	return Encode(n.Representation(), encFn)
}

// MarshalStreaming is like Marshal, but emits output to an io.Writer.
func MarshalStreaming(wr io.Writer, encFn Encoder, bind interface{}, typ schema.Type) error {
	n := bindnode.Wrap(bind, typ)
	return EncodeStreaming(wr, n.Representation(), encFn)
}

// Unmarshal accepts a pointer to a Go value and an IPLD schema type,
// and fills the value with data by decoding into it with the given Decoder function.
//
// Unmarshal uses the node/bindnode subsystem.
// See the documentation in that package for more details about its workings.
// Please note that this subsystem is relatively experimental at this time.
//
// The schema.Type parameter is optional, and can be nil.
// If given, it controls what kind of schema.Type (and what kind of representation strategy!)
// to use when processing the data.
// If absent, a default schema.Type will be inferred based on the golang type
// (so, a struct in go will be inferred to have a schema with a similar struct, and the default representation strategy (e.g. map), etc).
// Note that not all features of IPLD Schemas can be inferred from golang types alone.
// For example, to use union types, the schema parameter will be required.
// Similarly, to use most kinds of non-default representation strategy, the schema parameter is needed in order to convey that intention.
//
// In contrast to some other unmarshal conventions common in golang,
// notice that we also return a Node value.
// This Node points to the same data as the value you handed in as the bind parameter,
// while making it available to read and iterate and handle as a ipld datamodel.Node.
// If you don't need that interface, or intend to re-bind it later, you can discard that value.
//
// The 'bind' parameter may be nil.
// In that case, the type of the nil is still used to infer what kind of value to return,
// and a Node will still be returned based on that type.
// bindnode.Unwrap can be used on that Node and will still return something
// of the same golang type as the typed nil that was given as the 'bind' parameter.
func Unmarshal(b []byte, decFn Decoder, bind interface{}, typ schema.Type) (Node, error) {
	return UnmarshalStreaming(bytes.NewReader(b), decFn, bind, typ)
}

// UnmarshalStreaming is like Unmarshal, but works on an io.Reader for input.
func UnmarshalStreaming(r io.Reader, decFn Decoder, bind interface{}, typ schema.Type) (Node, error) {
	// Decode is fairly straightforward.
	np := bindnode.Prototype(bind, typ)
	n, err := DecodeStreamingUsingPrototype(r, decFn, np.Representation())
	if err != nil {
		return nil, err
	}
	// ... but our approach above allocated new memory, and we have to copy it back out.
	// In the future, the bindnode API could be improved to make this easier.
	if !reflect.ValueOf(bind).IsNil() {
		reflect.ValueOf(bind).Elem().Set(reflect.ValueOf(bindnode.Unwrap(n)).Elem())
	}
	// ... and we also have to re-bind a new node to the 'bind' value,
	// because probably the user will be surprised if mutating 'bind' doesn't affect the Node later.
	n = bindnode.Wrap(bind, typ)
	return n, err
}
