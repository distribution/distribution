// The datamodel package defines the most essential interfaces for describing IPLD Data --
// such as Node, NodePrototype, NodeBuilder, Link, and Path.
//
// Note that since interfaces in this package are the core of the library,
// choices made here maximize correctness and performance -- these choices
// are *not* always the choices that would maximize ergonomics.
// (Ergonomics can come on top; performance generally can't.)
// You'll want to check out other packages for functions with more ergonomics;
// for example, 'fluent' and its subpackages provide lots of ways to work with data;
// 'traversal' provides some ergonomic features for walking around data graphs;
// any use of schemas will provide a bunch of useful data validation options;
// or you can make your own function decorators that do what *you* need.
//
package datamodel
