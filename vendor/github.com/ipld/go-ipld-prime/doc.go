// go-ipld-prime is a series of go interfaces for manipulating IPLD data.
//
// See https://ipld.io/ for more information about the basics
// of "What is IPLD?".
//
// Here in the godoc, the first couple of types to look at should be:
//
//   - Node
//   - NodeBuilder and NodeAssembler
//   - NodePrototype.
//
// These types provide a generic description of the data model.
//
// A Node is a piece of IPLD data which can be inspected.
// A NodeAssembler is used to create Nodes.
// (A NodeBuilder is just like a NodeAssembler, but allocates memory
// (whereas a NodeAssembler just fills up memory; using these carefully
// allows construction of very efficient code.)
//
// Different NodePrototypes can be used to describe Nodes which follow certain logical rules
// (e.g., we use these as part of implementing Schemas),
// and can also be used so that programs can use different memory layouts for different data
// (which can be useful for constructing efficient programs when data has known shape for
// which we can use specific or compacted memory layouts).
//
// If working with linked data (data which is split into multiple
// trees of Nodes, loaded separately, and connected by some kind of
// "link" reference), the next types you should look at are:
//
//   - LinkSystem
//   - ... and its fields.
//
// The most typical use of LinkSystem is to use the linking/cid package
// to get a LinkSystem that works with CIDs:
//
//   lsys := cidlink.DefaultLinkSystem()
//
// ... and then assign the StorageWriteOpener and StorageReadOpener fields
// in order to control where data is stored to and read from.
// Methods on the LinkSystem then provide the functions typically used
// to get data in and out of Nodes so you can work with it.
//
// This root package gathers some of the most important ease-of-use functions
// all in one place, but is mostly aliases out to features originally found
// in other more specific sub-packages.  (If you're interested in keeping
// your binary sizes small, and don't use some of the features of this library,
// you'll probably want to look into using the relevant sub-packages directly.)
//
// Particularly interesting subpackages include:
//
//   - datamodel -- the most essential interfaces for describing data live here,
//        describing Node, NodePrototype, NodeBuilder, Link, and Path.
//   - node/* -- various Node + NodeBuilder implementations.
//   - node/basicnode -- the first Node implementation you should try.
//   - codec/* -- functions for serializing and deserializing Nodes.
//   - linking -- the LinkSystem, which is a facade to all data loading and storing and hashing.
//   - linking/* -- ways to bind concrete Link implementations (namely,
//        the linking/cidlink package, which connects the go-cid library to our datamodel.Link interface).
//   - traversal -- functions for walking Node graphs (including automatic link loading)
//        and visiting them programmatically.
//   - traversal/selector -- functions for working with IPLD Selectors,
//        which are a language-agnostic declarative format for describing graph walks.
//   - fluent/* -- various options for making datamodel Node and NodeBuilder easier to work with.
//   - schema -- interfaces for working with IPLD Schemas, which can bring constraints
//        and validation systems to otherwise schemaless and unstructured IPLD data.
//   - adl/* -- examples of creating and using Advanced Data Layouts (in short, custom Node implementations)
//        to do complex data structures transparently within the IPLD Data Model.
//
package ipld
