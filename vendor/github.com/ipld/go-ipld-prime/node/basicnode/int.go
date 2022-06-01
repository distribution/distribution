package basicnode

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = plainInt(0)
	_ datamodel.NodePrototype = Prototype__Int{}
	_ datamodel.NodeBuilder   = &plainInt__Builder{}
	_ datamodel.NodeAssembler = &plainInt__Assembler{}
)

func NewInt(value int64) datamodel.Node {
	v := plainInt(value)
	return &v
}

// plainInt is a simple boxed int that complies with datamodel.Node.
type plainInt int64

// -- Node interface methods -->

func (plainInt) Kind() datamodel.Kind {
	return datamodel.Kind_Int
}
func (plainInt) LookupByString(string) (datamodel.Node, error) {
	return mixins.Int{TypeName: "int"}.LookupByString("")
}
func (plainInt) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return mixins.Int{TypeName: "int"}.LookupByNode(nil)
}
func (plainInt) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Int{TypeName: "int"}.LookupByIndex(0)
}
func (plainInt) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return mixins.Int{TypeName: "int"}.LookupBySegment(seg)
}
func (plainInt) MapIterator() datamodel.MapIterator {
	return nil
}
func (plainInt) ListIterator() datamodel.ListIterator {
	return nil
}
func (plainInt) Length() int64 {
	return -1
}
func (plainInt) IsAbsent() bool {
	return false
}
func (plainInt) IsNull() bool {
	return false
}
func (plainInt) AsBool() (bool, error) {
	return mixins.Int{TypeName: "int"}.AsBool()
}
func (n plainInt) AsInt() (int64, error) {
	return int64(n), nil
}
func (plainInt) AsFloat() (float64, error) {
	return mixins.Int{TypeName: "int"}.AsFloat()
}
func (plainInt) AsString() (string, error) {
	return mixins.Int{TypeName: "int"}.AsString()
}
func (plainInt) AsBytes() ([]byte, error) {
	return mixins.Int{TypeName: "int"}.AsBytes()
}
func (plainInt) AsLink() (datamodel.Link, error) {
	return mixins.Int{TypeName: "int"}.AsLink()
}
func (plainInt) Prototype() datamodel.NodePrototype {
	return Prototype__Int{}
}

// -- NodePrototype -->

type Prototype__Int struct{}

func (Prototype__Int) NewBuilder() datamodel.NodeBuilder {
	var w plainInt
	return &plainInt__Builder{plainInt__Assembler{w: &w}}
}

// -- NodeBuilder -->

type plainInt__Builder struct {
	plainInt__Assembler
}

func (nb *plainInt__Builder) Build() datamodel.Node {
	return nb.w
}
func (nb *plainInt__Builder) Reset() {
	var w plainInt
	*nb = plainInt__Builder{plainInt__Assembler{w: &w}}
}

// -- NodeAssembler -->

type plainInt__Assembler struct {
	w *plainInt
}

func (plainInt__Assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return mixins.IntAssembler{TypeName: "int"}.BeginMap(0)
}
func (plainInt__Assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.IntAssembler{TypeName: "int"}.BeginList(0)
}
func (plainInt__Assembler) AssignNull() error {
	return mixins.IntAssembler{TypeName: "int"}.AssignNull()
}
func (plainInt__Assembler) AssignBool(bool) error {
	return mixins.IntAssembler{TypeName: "int"}.AssignBool(false)
}
func (na *plainInt__Assembler) AssignInt(v int64) error {
	*na.w = plainInt(v)
	return nil
}
func (plainInt__Assembler) AssignFloat(float64) error {
	return mixins.IntAssembler{TypeName: "int"}.AssignFloat(0)
}
func (plainInt__Assembler) AssignString(string) error {
	return mixins.IntAssembler{TypeName: "int"}.AssignString("")
}
func (plainInt__Assembler) AssignBytes([]byte) error {
	return mixins.IntAssembler{TypeName: "int"}.AssignBytes(nil)
}
func (plainInt__Assembler) AssignLink(datamodel.Link) error {
	return mixins.IntAssembler{TypeName: "int"}.AssignLink(nil)
}
func (na *plainInt__Assembler) AssignNode(v datamodel.Node) error {
	if v2, err := v.AsInt(); err != nil {
		return err
	} else {
		*na.w = plainInt(v2)
		return nil
	}
}
func (plainInt__Assembler) Prototype() datamodel.NodePrototype {
	return Prototype__Int{}
}
