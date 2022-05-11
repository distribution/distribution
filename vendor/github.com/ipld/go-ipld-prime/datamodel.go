package ipld

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/linking"
	"github.com/ipld/go-ipld-prime/schema"
)

type (
	Kind          = datamodel.Kind
	Node          = datamodel.Node
	NodeAssembler = datamodel.NodeAssembler
	NodeBuilder   = datamodel.NodeBuilder
	NodePrototype = datamodel.NodePrototype
	MapIterator   = datamodel.MapIterator
	MapAssembler  = datamodel.MapAssembler
	ListIterator  = datamodel.ListIterator
	ListAssembler = datamodel.ListAssembler

	Link          = datamodel.Link
	LinkPrototype = datamodel.LinkPrototype

	Path        = datamodel.Path
	PathSegment = datamodel.PathSegment
)

var (
	Null   = datamodel.Null
	Absent = datamodel.Absent
)

const (
	Kind_Invalid = datamodel.Kind_Invalid
	Kind_Map     = datamodel.Kind_Map
	Kind_List    = datamodel.Kind_List
	Kind_Null    = datamodel.Kind_Null
	Kind_Bool    = datamodel.Kind_Bool
	Kind_Int     = datamodel.Kind_Int
	Kind_Float   = datamodel.Kind_Float
	Kind_String  = datamodel.Kind_String
	Kind_Bytes   = datamodel.Kind_Bytes
	Kind_Link    = datamodel.Kind_Link
)

// Future: These aliases for the `KindSet_*` values may be dropped someday.
// I don't think they're very important to have cluttering up namespace here.
// They're included for a brief transitional period, largely for the sake of codegen things which have referred to them, but may disappear in the future.
var (
	KindSet_Recursive  = datamodel.KindSet_Recursive
	KindSet_Scalar     = datamodel.KindSet_Scalar
	KindSet_JustMap    = datamodel.KindSet_JustMap
	KindSet_JustList   = datamodel.KindSet_JustList
	KindSet_JustNull   = datamodel.KindSet_JustNull
	KindSet_JustBool   = datamodel.KindSet_JustBool
	KindSet_JustInt    = datamodel.KindSet_JustInt
	KindSet_JustFloat  = datamodel.KindSet_JustFloat
	KindSet_JustString = datamodel.KindSet_JustString
	KindSet_JustBytes  = datamodel.KindSet_JustBytes
	KindSet_JustLink   = datamodel.KindSet_JustLink
)

// Future: These error type aliases may be dropped someday.
// Being able to see them as having more than one package name is not helpful to clarity.
// They are left here for now for a brief transitional period, because it was relatively easy to do so.
type (
	ErrWrongKind             = datamodel.ErrWrongKind
	ErrNotExists             = datamodel.ErrNotExists
	ErrRepeatedMapKey        = datamodel.ErrRepeatedMapKey
	ErrInvalidSegmentForList = datamodel.ErrInvalidSegmentForList
	ErrIteratorOverread      = datamodel.ErrIteratorOverread
	ErrInvalidKey            = schema.ErrInvalidKey
	ErrMissingRequiredField  = schema.ErrMissingRequiredField
	ErrHashMismatch          = linking.ErrHashMismatch
)

// Future: a bunch of these alias methods for path creation may be dropped someday.
// They don't hurt anything, but I don't think they add much clarity either, vs the amount of namespace noise they create;
// many of the high level convenience functions we add here in the root package will probably refer to datamodel.Path, and that should be sufficient to create clarity for new users for where to look for more on pathing.
// They are here for now for a transitional period, but may eventually be revisited and perhaps removed.

// NewPath is an alias for datamodel.NewPath.
//
// Pathing is a concept defined in the data model layer of IPLD.
func NewPath(segments []PathSegment) Path {
	return datamodel.NewPath(segments)
}

// ParsePath is an alias for datamodel.ParsePath.
//
// Pathing is a concept defined in the data model layer of IPLD.
func ParsePath(pth string) Path {
	return datamodel.ParsePath(pth)
}

// ParsePathSegment is an alias for datamodel.ParsePathSegment.
//
// Pathing is a concept defined in the data model layer of IPLD.
func ParsePathSegment(s string) PathSegment {
	return datamodel.ParsePathSegment(s)
}

// PathSegmentOfString is an alias for datamodel.PathSegmentOfString.
//
// Pathing is a concept defined in the data model layer of IPLD.
func PathSegmentOfString(s string) PathSegment {
	return datamodel.PathSegmentOfString(s)
}

// PathSegmentOfInt is an alias for datamodel.PathSegmentOfInt.
//
// Pathing is a concept defined in the data model layer of IPLD.
func PathSegmentOfInt(i int64) PathSegment {
	return datamodel.PathSegmentOfInt(i)
}
