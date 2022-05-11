package basicnode

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = plainFloat(0)
	_ datamodel.NodePrototype = Prototype__Float{}
	_ datamodel.NodeBuilder   = &plainFloat__Builder{}
	_ datamodel.NodeAssembler = &plainFloat__Assembler{}
)

func NewFloat(value float64) datamodel.Node {
	v := plainFloat(value)
	return &v
}

// plainFloat is a simple boxed float that complies with datamodel.Node.
type plainFloat float64

// -- Node interface methods -->

func (plainFloat) Kind() datamodel.Kind {
	return datamodel.Kind_Float
}
func (plainFloat) LookupByString(string) (datamodel.Node, error) {
	return mixins.Float{TypeName: "float"}.LookupByString("")
}
func (plainFloat) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return mixins.Float{TypeName: "float"}.LookupByNode(nil)
}
func (plainFloat) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Float{TypeName: "float"}.LookupByIndex(0)
}
func (plainFloat) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return mixins.Float{TypeName: "float"}.LookupBySegment(seg)
}
func (plainFloat) MapIterator() datamodel.MapIterator {
	return nil
}
func (plainFloat) ListIterator() datamodel.ListIterator {
	return nil
}
func (plainFloat) Length() int64 {
	return -1
}
func (plainFloat) IsAbsent() bool {
	return false
}
func (plainFloat) IsNull() bool {
	return false
}
func (plainFloat) AsBool() (bool, error) {
	return mixins.Float{TypeName: "float"}.AsBool()
}
func (plainFloat) AsInt() (int64, error) {
	return mixins.Float{TypeName: "float"}.AsInt()
}
func (n plainFloat) AsFloat() (float64, error) {
	return float64(n), nil
}
func (plainFloat) AsString() (string, error) {
	return mixins.Float{TypeName: "float"}.AsString()
}
func (plainFloat) AsBytes() ([]byte, error) {
	return mixins.Float{TypeName: "float"}.AsBytes()
}
func (plainFloat) AsLink() (datamodel.Link, error) {
	return mixins.Float{TypeName: "float"}.AsLink()
}
func (plainFloat) Prototype() datamodel.NodePrototype {
	return Prototype__Float{}
}

// -- NodePrototype -->

type Prototype__Float struct{}

func (Prototype__Float) NewBuilder() datamodel.NodeBuilder {
	var w plainFloat
	return &plainFloat__Builder{plainFloat__Assembler{w: &w}}
}

// -- NodeBuilder -->

type plainFloat__Builder struct {
	plainFloat__Assembler
}

func (nb *plainFloat__Builder) Build() datamodel.Node {
	return nb.w
}
func (nb *plainFloat__Builder) Reset() {
	var w plainFloat
	*nb = plainFloat__Builder{plainFloat__Assembler{w: &w}}
}

// -- NodeAssembler -->

type plainFloat__Assembler struct {
	w *plainFloat
}

func (plainFloat__Assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return mixins.FloatAssembler{TypeName: "float"}.BeginMap(0)
}
func (plainFloat__Assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.FloatAssembler{TypeName: "float"}.BeginList(0)
}
func (plainFloat__Assembler) AssignNull() error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignNull()
}
func (plainFloat__Assembler) AssignBool(bool) error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignBool(false)
}
func (plainFloat__Assembler) AssignInt(int64) error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignInt(0)
}
func (na *plainFloat__Assembler) AssignFloat(v float64) error {
	*na.w = plainFloat(v)
	return nil
}
func (plainFloat__Assembler) AssignString(string) error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignString("")
}
func (plainFloat__Assembler) AssignBytes([]byte) error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignBytes(nil)
}
func (plainFloat__Assembler) AssignLink(datamodel.Link) error {
	return mixins.FloatAssembler{TypeName: "float"}.AssignLink(nil)
}
func (na *plainFloat__Assembler) AssignNode(v datamodel.Node) error {
	if v2, err := v.AsFloat(); err != nil {
		return err
	} else {
		*na.w = plainFloat(v2)
		return nil
	}
}
func (plainFloat__Assembler) Prototype() datamodel.NodePrototype {
	return Prototype__Float{}
}
