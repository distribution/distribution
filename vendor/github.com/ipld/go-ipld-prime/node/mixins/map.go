package mixins

import (
	"github.com/ipld/go-ipld-prime/datamodel"
)

// Map can be embedded in a struct to provide all the methods that
// have fixed output for any map-kinded nodes.
// (Mostly this includes all the methods which simply return ErrWrongKind.)
// Other methods will still need to be implemented to finish conforming to Node.
//
// To conserve memory and get a TypeName in errors without embedding,
// write methods on your type with a body that simply initializes this struct
// and immediately uses the relevant method;
// this is more verbose in source, but compiles to a tighter result:
// in memory, there's no embed; and in runtime, the calls will be inlined
// and thus have no cost in execution time.
type Map struct {
	TypeName string
}

func (Map) Kind() datamodel.Kind {
	return datamodel.Kind_Map
}
func (x Map) LookupByIndex(idx int64) (datamodel.Node, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "LookupByIndex", AppropriateKind: datamodel.KindSet_JustList, ActualKind: datamodel.Kind_Map}
}
func (Map) ListIterator() datamodel.ListIterator {
	return nil
}
func (Map) IsAbsent() bool {
	return false
}
func (Map) IsNull() bool {
	return false
}
func (x Map) AsBool() (bool, error) {
	return false, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_Map}
}
func (x Map) AsInt() (int64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_Map}
}
func (x Map) AsFloat() (float64, error) {
	return 0, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_Map}
}
func (x Map) AsString() (string, error) {
	return "", datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsString", AppropriateKind: datamodel.KindSet_JustString, ActualKind: datamodel.Kind_Map}
}
func (x Map) AsBytes() ([]byte, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_Map}
}
func (x Map) AsLink() (datamodel.Link, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AsLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_Map}
}

// MapAssembler has similar purpose as Map, but for (you guessed it)
// the NodeAssembler interface rather than the Node interface.
type MapAssembler struct {
	TypeName string
}

func (x MapAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return nil, datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "BeginList", AppropriateKind: datamodel.KindSet_JustList, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignNull() error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignNull", AppropriateKind: datamodel.KindSet_JustNull, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignBool(bool) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBool", AppropriateKind: datamodel.KindSet_JustBool, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignInt(int64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignInt", AppropriateKind: datamodel.KindSet_JustInt, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignFloat(float64) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignFloat", AppropriateKind: datamodel.KindSet_JustFloat, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignString(string) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignString", AppropriateKind: datamodel.KindSet_JustString, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignBytes([]byte) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignBytes", AppropriateKind: datamodel.KindSet_JustBytes, ActualKind: datamodel.Kind_Map}
}
func (x MapAssembler) AssignLink(datamodel.Link) error {
	return datamodel.ErrWrongKind{TypeName: x.TypeName, MethodName: "AssignLink", AppropriateKind: datamodel.KindSet_JustLink, ActualKind: datamodel.Kind_Map}
}
