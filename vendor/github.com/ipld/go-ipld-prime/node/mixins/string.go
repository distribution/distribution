package mixins

import (
	"github.com/ipld/go-ipld-prime/datamodel"
)

// String can be embedded in a struct to provide all the methods that
// have fixed output for any string-kinded nodes.
// (Mostly this includes all the methods which simply return ErrWrongKind.)
// Other methods will still need to be implemented to finish conforming to Node.
//
// To conserve memory and get a TypeName in errors without embedding,
// write methods on your type with a body that simply initializes this struct
// and immediately uses the relevant method;
// this is more verbose in source, but compiles to a tighter result:
// in memory, there's no embed; and in runtime, the calls will be inlined
// and thus have no cost in execution time.
type String struct {
	TypeName string
}

func (String) Kind() datamodel.Kind {
	return datamodel.Kind_String
}
func (x String) LookupByString(string) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByString", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_String}
}
func (x String) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByNode", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_String}
}
func (x String) LookupByIndex(idx int64) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByIndex", AppropriateKind: datamodel.KindSet_JustList, ActualKind: datamodel.Kind_String}
}
func (x String) LookupBySegment(datamodel.PathSegment) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupBySegment", AppropriateKind: datamodel.KindSet_Recursive, ActualKind: datamodel.Kind_String}
}
func (String) MapIterator() datamodel.MapIterator {
	return nil
}
func (String) ListIterator() datamodel.ListIterator {
	return nil
}
func (String) Length() int64 {
	return -1
}
func (String) IsAbsent() bool {
	return false
}
func (String) IsNull() bool {
	return false
}
func (x String) AsBool() (bool, error) {
	return false, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_String}
}
func (x String) AsInt() (int64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_String}
}
func (x String) AsFloat() (float64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_String}
}
func (x String) AsBytes() ([]byte, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_String}
}
func (x String) AsLink() (datamodel.Link, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_String}
}

// StringAssembler has similar purpose as String, but for (you guessed it)
// the NodeAssembler interface rather than the Node interface.
type StringAssembler struct {
	TypeName string
}

func (x StringAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "BeginMap", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "BeginList", AppropriateKind: datamodel.KindSet_JustList, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignNull() error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignNull", AppropriateKind: datamodel.KindSet_JustNull, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignBool(bool) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignInt(int64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignFloat(float64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignBytes([]byte) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_String}
}
func (x StringAssembler) AssignLink(datamodel.Link) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_String}
}
