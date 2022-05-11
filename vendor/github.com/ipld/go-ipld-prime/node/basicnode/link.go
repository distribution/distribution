package basicnode

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = &plainLink{}
	_ datamodel.NodePrototype = Prototype__Link{}
	_ datamodel.NodeBuilder   = &plainLink__Builder{}
	_ datamodel.NodeAssembler = &plainLink__Assembler{}
)

func NewLink(value datamodel.Link) datamodel.Node {
	return &plainLink{value}
}

// plainLink is a simple box around a Link that complies with datamodel.Node.
type plainLink struct {
	x datamodel.Link
}

// -- Node interface methods -->

func (plainLink) Kind() datamodel.Kind {
	return datamodel.Kind_Link
}
func (plainLink) LookupByString(string) (datamodel.Node, error) {
	return mixins.Link{TypeName: "link"}.LookupByString("")
}
func (plainLink) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return mixins.Link{TypeName: "link"}.LookupByNode(nil)
}
func (plainLink) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Link{TypeName: "link"}.LookupByIndex(0)
}
func (plainLink) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return mixins.Link{TypeName: "link"}.LookupBySegment(seg)
}
func (plainLink) MapIterator() datamodel.MapIterator {
	return nil
}
func (plainLink) ListIterator() datamodel.ListIterator {
	return nil
}
func (plainLink) Length() int64 {
	return -1
}
func (plainLink) IsAbsent() bool {
	return false
}
func (plainLink) IsNull() bool {
	return false
}
func (plainLink) AsBool() (bool, error) {
	return mixins.Link{TypeName: "link"}.AsBool()
}
func (plainLink) AsInt() (int64, error) {
	return mixins.Link{TypeName: "link"}.AsInt()
}
func (plainLink) AsFloat() (float64, error) {
	return mixins.Link{TypeName: "link"}.AsFloat()
}
func (plainLink) AsString() (string, error) {
	return mixins.Link{TypeName: "link"}.AsString()
}
func (plainLink) AsBytes() ([]byte, error) {
	return mixins.Link{TypeName: "link"}.AsBytes()
}
func (n *plainLink) AsLink() (datamodel.Link, error) {
	return n.x, nil
}
func (plainLink) Prototype() datamodel.NodePrototype {
	return Prototype__Link{}
}

// -- NodePrototype -->

type Prototype__Link struct{}

func (Prototype__Link) NewBuilder() datamodel.NodeBuilder {
	var w plainLink
	return &plainLink__Builder{plainLink__Assembler{w: &w}}
}

// -- NodeBuilder -->

type plainLink__Builder struct {
	plainLink__Assembler
}

func (nb *plainLink__Builder) Build() datamodel.Node {
	return nb.w
}
func (nb *plainLink__Builder) Reset() {
	var w plainLink
	*nb = plainLink__Builder{plainLink__Assembler{w: &w}}
}

// -- NodeAssembler -->

type plainLink__Assembler struct {
	w *plainLink
}

func (plainLink__Assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return mixins.LinkAssembler{TypeName: "link"}.BeginMap(0)
}
func (plainLink__Assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.LinkAssembler{TypeName: "link"}.BeginList(0)
}
func (plainLink__Assembler) AssignNull() error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignNull()
}
func (plainLink__Assembler) AssignBool(bool) error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignBool(false)
}
func (plainLink__Assembler) AssignInt(int64) error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignInt(0)
}
func (plainLink__Assembler) AssignFloat(float64) error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignFloat(0)
}
func (plainLink__Assembler) AssignString(string) error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignString("")
}
func (plainLink__Assembler) AssignBytes([]byte) error {
	return mixins.LinkAssembler{TypeName: "link"}.AssignBytes(nil)
}
func (na *plainLink__Assembler) AssignLink(v datamodel.Link) error {
	na.w.x = v
	return nil
}
func (na *plainLink__Assembler) AssignNode(v datamodel.Node) error {
	if v2, err := v.AsLink(); err != nil {
		return err
	} else {
		na.w.x = v2
		return nil
	}
}
func (plainLink__Assembler) Prototype() datamodel.NodePrototype {
	return Prototype__Link{}
}
