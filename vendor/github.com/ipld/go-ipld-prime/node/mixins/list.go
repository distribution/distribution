package mixins

import (
	"github.com/ipld/go-ipld-prime/datamodel"
)

// List can be embedded in a struct to provide all the methods that
// have fixed output for any int-kinded nodes.
// (Mostly this includes all the methods which simply return ErrWrongKind.)
// Other methods will still need to be implemented to finish conforming to Node.
//
// To conserve memory and get a TypeName in errors without embedding,
// write methods on your type with a body that simply initializes this struct
// and immediately uses the relevant method;
// this is more verbose in source, but compiles to a tighter result:
// in memory, there's no embed; and in runtime, the calls will be inlined
// and thus have no cost in execution time.
type List struct {
	TypeName string
}

func (List) Kind() datamodel.Kind {
	return datamodel.Kind_List
}
func (x List) LookupByString(string) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByString", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_List}
}
func (x List) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByNode", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_List}
}
func (List) MapIterator() datamodel.MapIterator {
	return nil
}
func (List) IsAbsent() bool {
	return false
}
func (List) IsNull() bool {
	return false
}
func (x List) AsBool() (bool, error) {
	return false, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_List}
}
func (x List) AsInt() (int64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_List}
}
func (x List) AsFloat() (float64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_List}
}
func (x List) AsString() (string, error) {
	return "", datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsString", AppropriateKind: datamodel.KindSet_JustString, ActualKind: datamodel.Kind_List}
}
func (x List) AsBytes() ([]byte, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_List}
}
func (x List) AsLink() (datamodel.Link, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_List}
}

// ListAssembler has similar purpose as List, but for (you guessed it)
// the NodeAssembler interface rather than the Node interface.
type ListAssembler struct {
	TypeName string
}

func (x ListAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "BeginMap", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignNull() error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignNull", AppropriateKind: datamodel.KindSet_JustNull, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignBool(bool) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignInt(int64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignFloat(float64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignString(string) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignString", AppropriateKind: datamodel.KindSet_JustString, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignBytes([]byte) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_List}
}
func (x ListAssembler) AssignLink(datamodel.Link) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_List}
}
