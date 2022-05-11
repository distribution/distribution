package datamodel

// Null is the one kind of node we can have a true singleton implementation of.
// This is that value.
// The Null Node has Kind() == Kind_Null, returns IsNull() == true,
// returns ErrWrongKind to most other inquiries (as you'd expect),
// and returns a NodePrototype with a NewBuilder method that simply panics
// (because why would you ever try to build a new "null"?).
var Null Node = nullNode{}

type nullNode struct{}

func (nullNode) Kind() Kind {
	return Kind_Null
}
func (nullNode) LookupByString(key string) (Node, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "LookupByString", AppropriateKind: KindSet_JustMap, ActualKind: Kind_Null}
}
func (nullNode) LookupByNode(key Node) (Node, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "LookupByNode", AppropriateKind: KindSet_JustMap, ActualKind: Kind_Null}
}
func (nullNode) LookupByIndex(idx int64) (Node, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "LookupByIndex", AppropriateKind: KindSet_JustList, ActualKind: Kind_Null}
}
func (nullNode) LookupBySegment(seg PathSegment) (Node, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "LookupBySegment", AppropriateKind: KindSet_Recursive, ActualKind: Kind_Null}
}
func (nullNode) MapIterator() MapIterator {
	return nil
}
func (nullNode) ListIterator() ListIterator {
	return nil
}
func (nullNode) Length() int64 {
	return -1
}
func (nullNode) IsAbsent() bool {
	return false
}
func (nullNode) IsNull() bool {
	return true
}
func (nullNode) AsBool() (bool, error) {
	return false, ErrWrongKind{TypeName: "null", MethodName: "AsBool", AppropriateKind: KindSet_JustBool, ActualKind: Kind_Null}
}
func (nullNode) AsInt() (int64, error) {
	return 0, ErrWrongKind{TypeName: "null", MethodName: "AsInt", AppropriateKind: KindSet_JustInt, ActualKind: Kind_Null}
}
func (nullNode) AsFloat() (float64, error) {
	return 0, ErrWrongKind{TypeName: "null", MethodName: "AsFloat", AppropriateKind: KindSet_JustFloat, ActualKind: Kind_Null}
}
func (nullNode) AsString() (string, error) {
	return "", ErrWrongKind{TypeName: "null", MethodName: "AsString", AppropriateKind: KindSet_JustString, ActualKind: Kind_Null}
}
func (nullNode) AsBytes() ([]byte, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "AsBytes", AppropriateKind: KindSet_JustBytes, ActualKind: Kind_Null}
}
func (nullNode) AsLink() (Link, error) {
	return nil, ErrWrongKind{TypeName: "null", MethodName: "AsLink", AppropriateKind: KindSet_JustLink, ActualKind: Kind_Null}
}
func (nullNode) Prototype() NodePrototype {
	return nullPrototype{}
}

type nullPrototype struct{}

func (nullPrototype) NewBuilder() NodeBuilder {
	panic("cannot build null nodes")
}

// Absent is the _other_ kind of node (besides Null) we can have a true singleton implementation of.
// This is the singleton value for Absent.
// The Absent Node has Kind() == Kind_Null, returns IsNull() == false (!),
// returns IsAbsent() == true,
// returns ErrWrongKind to most other inquiries (as you'd expect),
// and returns a NodePrototype with a NewBuilder method that simply panics
// (because why would you ever try to build a new "nothing"?).
//
// Despite its presence in the datamodel package, "absent" is not really a data model concept.
// Absent should not really be seen in any datamodel Node implementations, for example.
// Absent is seen used in the realm of schemas and typed data, because there,
// there's a concept of data that has been described, and yet is not currently present;
// it is this concept that "absent" is used to express.
// Absent also sometimes shows up as a distinct case in codecs or other diagnostic printing,
// and suchlike miscellaneous places; it is for these reasons that it's here in the datamodel package,
// because it would end up imported somewhat universally for those diagnostic purposes anyway.
// (This may be worth a design review at some point, but holds up well enough for now.)
var Absent Node = absentNode{}

type absentNode struct{}

func (absentNode) Kind() Kind {
	return Kind_Null
}
func (absentNode) LookupByString(key string) (Node, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "LookupByString", AppropriateKind: KindSet_JustMap, ActualKind: Kind_Null}
}
func (absentNode) LookupByNode(key Node) (Node, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "LookupByNode", AppropriateKind: KindSet_JustMap, ActualKind: Kind_Null}
}
func (absentNode) LookupByIndex(idx int64) (Node, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "LookupByIndex", AppropriateKind: KindSet_JustList, ActualKind: Kind_Null}
}
func (absentNode) LookupBySegment(seg PathSegment) (Node, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "LookupBySegment", AppropriateKind: KindSet_Recursive, ActualKind: Kind_Null}
}
func (absentNode) MapIterator() MapIterator {
	return nil
}
func (absentNode) ListIterator() ListIterator {
	return nil
}
func (absentNode) Length() int64 {
	return -1
}
func (absentNode) IsAbsent() bool {
	return true
}
func (absentNode) IsNull() bool {
	return false
}
func (absentNode) AsBool() (bool, error) {
	return false, ErrWrongKind{TypeName: "absent", MethodName: "AsBool", AppropriateKind: KindSet_JustBool, ActualKind: Kind_Null}
}
func (absentNode) AsInt() (int64, error) {
	return 0, ErrWrongKind{TypeName: "absent", MethodName: "AsInt", AppropriateKind: KindSet_JustInt, ActualKind: Kind_Null}
}
func (absentNode) AsFloat() (float64, error) {
	return 0, ErrWrongKind{TypeName: "absent", MethodName: "AsFloat", AppropriateKind: KindSet_JustFloat, ActualKind: Kind_Null}
}
func (absentNode) AsString() (string, error) {
	return "", ErrWrongKind{TypeName: "absent", MethodName: "AsString", AppropriateKind: KindSet_JustString, ActualKind: Kind_Null}
}
func (absentNode) AsBytes() ([]byte, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "AsBytes", AppropriateKind: KindSet_JustBytes, ActualKind: Kind_Null}
}
func (absentNode) AsLink() (Link, error) {
	return nil, ErrWrongKind{TypeName: "absent", MethodName: "AsLink", AppropriateKind: KindSet_JustLink, ActualKind: Kind_Null}
}
func (absentNode) Prototype() NodePrototype {
	return absentPrototype{}
}

type absentPrototype struct{}

func (absentPrototype) NewBuilder() NodeBuilder {
	panic("cannot build absent nodes")
}
