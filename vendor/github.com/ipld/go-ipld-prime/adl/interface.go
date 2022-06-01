package adl

import (
	"github.com/ipld/go-ipld-prime/datamodel"
)

// ADL is an interface denoting an Advanced Data Layout,
// which is something that supports all the datamodel.Node operations,
// but may be doing so using some custom internal logic.
//
// For more details, see the docs at
// https://ipld.io/docs/advanced-data-layouts/ .
//
// This interface doesn't specify much new behavior, but it does include
// the requirement of a way to tell an examiner about your "substrate",
// since this concept does seem to be present in all ADLs.
type ADL interface {
	datamodel.Node

	// Substrate returns the underlying Data Model node, which can be used
	// to encode an ADL's raw layout.
	//
	// Note that the substrate of an ADL can contain other ADLs!
	Substrate() datamodel.Node
}
