package datamodel

import (
	"fmt"
)

// ErrWrongKind may be returned from functions on the Node interface when
// a method is invoked which doesn't make sense for the Kind that node
// concretely contains.
//
// For example, calling AsString on a map will return ErrWrongKind.
// Calling Lookup on an int will similarly return ErrWrongKind.
type ErrWrongKind struct {
	// TypeName may optionally indicate the named type of a node the function
	// was called on (if the node was typed!), or, may be the empty string.
	TypeName string

	// MethodName is literally the string for the operation attempted, e.g.
	// "AsString".
	//
	// For methods on nodebuilders, we say e.g. "NodeBuilder.CreateMap".
	MethodName string

	// ApprorpriateKind describes which Kinds the erroring method would
	// make sense for.
	AppropriateKind KindSet

	// ActualKind describes the Kind of the node the method was called on.
	//
	// In the case of typed nodes, this will typically refer to the 'natural'
	// data-model kind for such a type (e.g., structs will say 'map' here).
	ActualKind Kind

	// TODO: it may be desirable for this error to be able to describe the schema typekind, too, if applicable.
	// Of course this presents certain package import graph problems.  Solution to this that maximizes user usability is unclear.
}

func (e ErrWrongKind) Error() string {
	if e.TypeName == "" {
		return fmt.Sprintf("func called on wrong kind: %q called on a %s node, but only makes sense on %s", e.MethodName, e.ActualKind, e.AppropriateKind)
	} else {
		return fmt.Sprintf("func called on wrong kind: %q called on a %s node (kind: %s), but only makes sense on %s", e.MethodName, e.TypeName, e.ActualKind, e.AppropriateKind)
	}
}

// TODO: revisit the claim below about ErrNoSuchField.  I think we moved back away from that, or want to.

// ErrNotExists may be returned from the lookup functions of the Node interface
// to indicate a missing value.
//
// Note that schema.ErrNoSuchField is another type of error which sometimes
// occurs in similar places as ErrNotExists.  ErrNoSuchField is preferred
// when handling data with constraints provided by a schema that mean that
// a field can *never* exist (as differentiated from a map key which is
// simply absent in some data).
type ErrNotExists struct {
	Segment PathSegment
}

func (e ErrNotExists) Error() string {
	return fmt.Sprintf("key not found: %q", e.Segment)
}

// ErrRepeatedMapKey is an error indicating that a key was inserted
// into a map that already contains that key.
//
// This error may be returned by any methods that add data to a map --
// any of the methods on a NodeAssembler that was yielded by MapAssembler.AssignKey(),
// or from the MapAssembler.AssignDirectly() method.
type ErrRepeatedMapKey struct {
	Key Node
}

func (e ErrRepeatedMapKey) Error() string {
	return fmt.Sprintf("cannot repeat map key %q", e.Key)
}

// ErrInvalidSegmentForList is returned when using Node.LookupBySegment and the
// given PathSegment can't be applied to a list because it's unparsable as a number.
type ErrInvalidSegmentForList struct {
	// TypeName may indicate the named type of a node the function was called on,
	// or be empty string if working on untyped data.
	TypeName string

	// TroubleSegment is the segment we couldn't use.
	TroubleSegment PathSegment

	// Reason may explain more about why the PathSegment couldn't be used;
	// in practice, it's probably a 'strconv.NumError'.
	Reason error
}

func (e ErrInvalidSegmentForList) Error() string {
	v := "invalid segment for lookup on a list"
	if e.TypeName != "" {
		v += " of type " + e.TypeName
	}
	return v + fmt.Sprintf(": %q: %s", e.TroubleSegment.s, e.Reason)
}

// ErrIteratorOverread is returned when calling 'Next' on a MapIterator or
// ListIterator when it is already done.
type ErrIteratorOverread struct{}

func (e ErrIteratorOverread) Error() string {
	return "iterator overread"
}
