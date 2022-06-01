// Package bindnode provides a datamodel.Node implementation via Go reflection.
//
// This package is EXPERIMENTAL; its behavior and API might change as it's still
// in development.
package bindnode

import (
	"reflect"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/schema"
)

// Prototype implements a schema.TypedPrototype given a Go pointer type and an
// IPLD schema type. Note that the result is also a datamodel.NodePrototype.
//
// If both the Go type and schema type are supplied, it is assumed that they are
// compatible with one another.
//
// If either the Go type or schema type are nil, we infer the missing type from
// the other provided type. For example, we can infer an unnamed Go struct type
// for a schema struct type, and we can infer a schema Int type for a Go int64
// type. The inferring logic is still a work in progress and subject to change.
// At this time, inferring IPLD Unions and Enums from Go types is not supported.
//
// When supplying a non-nil ptrType, Prototype only obtains the Go pointer type
// from it, so its underlying value will typically be nil. For example:
//
//     proto := bindnode.Prototype((*goType)(nil), schemaType)
func Prototype(ptrType interface{}, schemaType schema.Type) schema.TypedPrototype {
	if ptrType == nil && schemaType == nil {
		panic("bindnode: either ptrType or schemaType must not be nil")
	}

	// TODO: if both are supplied, verify that they are compatible

	var goType reflect.Type
	if ptrType == nil {
		goType = inferGoType(schemaType)
	} else {
		goPtrType := reflect.TypeOf(ptrType)
		if goPtrType.Kind() != reflect.Ptr {
			panic("bindnode: ptrType must be a pointer")
		}
		goType = goPtrType.Elem()

		if schemaType == nil {
			schemaType = inferSchema(goType)
		} else {
			verifyCompatibility(make(map[seenEntry]bool), goType, schemaType)
		}
	}

	return &_prototype{schemaType: schemaType, goType: goType}
}

// Wrap implements a schema.TypedNode given a non-nil pointer to a Go value and an
// IPLD schema type. Note that the result is also a datamodel.Node.
//
// Wrap is meant to be used when one already has a Go value with data.
// As such, ptrVal must not be nil.
//
// Similar to Prototype, if schemaType is non-nil it is assumed to be compatible
// with the Go type, and otherwise it's inferred from the Go type.
func Wrap(ptrVal interface{}, schemaType schema.Type) schema.TypedNode {
	if ptrVal == nil {
		panic("bindnode: ptrVal must not be nil")
	}
	goPtrVal := reflect.ValueOf(ptrVal)
	if goPtrVal.Kind() != reflect.Ptr {
		panic("bindnode: ptrVal must be a pointer")
	}
	if goPtrVal.IsNil() {
		// Note that this can happen if ptrVal was a typed nil.
		panic("bindnode: ptrVal must not be nil")
	}
	goVal := goPtrVal.Elem()
	if schemaType == nil {
		schemaType = inferSchema(goVal.Type())
	} else {
		verifyCompatibility(make(map[seenEntry]bool), goVal.Type(), schemaType)
	}
	return &_node{val: goVal, schemaType: schemaType}
}

// TODO: consider making our own Node interface, like:
//
// type WrappedNode interface {
//     datamodel.Node
//     Unwrap() (ptrVal interface)
// }
//
// Pros: API is easier to understand, harder to mix up with other datamodel.Nodes.
// Cons: One usually only has a datamodel.Node, and type assertions can be weird.

// Unwrap takes a datamodel.Node implemented by Prototype or Wrap,
// and returns a pointer to the inner Go value.
//
// Unwrap returns nil if the node isn't implemented by this package.
func Unwrap(node datamodel.Node) (ptrVal interface{}) {
	var val reflect.Value
	switch node := node.(type) {
	case *_node:
		val = node.val
	case *_nodeRepr:
		val = node.val
	default:
		return nil
	}
	if val.Kind() == reflect.Ptr {
		panic("bindnode: didn't expect val to be a pointer")
	}
	return val.Addr().Interface()
}
