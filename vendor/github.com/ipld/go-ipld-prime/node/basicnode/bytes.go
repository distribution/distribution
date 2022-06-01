package basicnode

import (
	"bytes"
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = plainBytes(nil)
	_ datamodel.NodePrototype = Prototype__Bytes{}
	_ datamodel.NodeBuilder   = &plainBytes__Builder{}
	_ datamodel.NodeAssembler = &plainBytes__Assembler{}
)

func NewBytes(value []byte) datamodel.Node {
	v := plainBytes(value)
	return &v
}

// plainBytes is a simple boxed byte slice that complies with datamodel.Node.
type plainBytes []byte

// -- Node interface methods -->

func (plainBytes) Kind() datamodel.Kind {
	return datamodel.Kind_Bytes
}
func (plainBytes) LookupByString(string) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByString("")
}
func (plainBytes) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByNode(nil)
}
func (plainBytes) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByIndex(0)
}
func (plainBytes) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupBySegment(seg)
}
func (plainBytes) MapIterator() datamodel.MapIterator {
	return nil
}
func (plainBytes) ListIterator() datamodel.ListIterator {
	return nil
}
func (plainBytes) Length() int64 {
	return -1
}
func (plainBytes) IsAbsent() bool {
	return false
}
func (plainBytes) IsNull() bool {
	return false
}
func (plainBytes) AsBool() (bool, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsBool()
}
func (plainBytes) AsInt() (int64, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsInt()
}
func (plainBytes) AsFloat() (float64, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsFloat()
}
func (plainBytes) AsString() (string, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsString()
}
func (n plainBytes) AsBytes() ([]byte, error) {
	return []byte(n), nil
}
func (plainBytes) AsLink() (datamodel.Link, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsLink()
}
func (plainBytes) Prototype() datamodel.NodePrototype {
	return Prototype__Bytes{}
}
func (n plainBytes) AsLargeBytes() (io.ReadSeeker, error) {
	return bytes.NewReader(n), nil
}

// -- NodePrototype -->

type Prototype__Bytes struct{}

func (Prototype__Bytes) NewBuilder() datamodel.NodeBuilder {
	var w plainBytes
	return &plainBytes__Builder{plainBytes__Assembler{w: &w}}
}

// -- NodeBuilder -->

type plainBytes__Builder struct {
	plainBytes__Assembler
}

func (nb *plainBytes__Builder) Build() datamodel.Node {
	return nb.w
}
func (nb *plainBytes__Builder) Reset() {
	var w plainBytes
	*nb = plainBytes__Builder{plainBytes__Assembler{w: &w}}
}

// -- NodeAssembler -->

type plainBytes__Assembler struct {
	w datamodel.Node
}

func (plainBytes__Assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return mixins.BytesAssembler{TypeName: "bytes"}.BeginMap(0)
}
func (plainBytes__Assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.BytesAssembler{TypeName: "bytes"}.BeginList(0)
}
func (plainBytes__Assembler) AssignNull() error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignNull()
}
func (plainBytes__Assembler) AssignBool(bool) error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignBool(false)
}
func (plainBytes__Assembler) AssignInt(int64) error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignInt(0)
}
func (plainBytes__Assembler) AssignFloat(float64) error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignFloat(0)
}
func (plainBytes__Assembler) AssignString(string) error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignString("")
}
func (na *plainBytes__Assembler) AssignBytes(v []byte) error {
	na.w = datamodel.Node(plainBytes(v))
	return nil
}
func (plainBytes__Assembler) AssignLink(datamodel.Link) error {
	return mixins.BytesAssembler{TypeName: "bytes"}.AssignLink(nil)
}
func (na *plainBytes__Assembler) AssignNode(v datamodel.Node) error {
	if lb, ok := v.(datamodel.LargeBytesNode); ok {
		lbn, err := lb.AsLargeBytes()
		if err == nil {
			na.w = streamBytes{lbn}
			return nil
		}
	}
	if v2, err := v.AsBytes(); err != nil {
		return err
	} else {
		na.w = plainBytes(v2)
		return nil
	}
}
func (plainBytes__Assembler) Prototype() datamodel.NodePrototype {
	return Prototype__Bytes{}
}
