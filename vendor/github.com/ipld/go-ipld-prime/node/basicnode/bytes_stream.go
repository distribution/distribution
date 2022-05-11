package basicnode

import (
	"io"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = streamBytes{nil}
	_ datamodel.NodePrototype = Prototype__Bytes{}
	_ datamodel.NodeBuilder   = &plainBytes__Builder{}
	_ datamodel.NodeAssembler = &plainBytes__Assembler{}
)

func NewBytesFromReader(rs io.ReadSeeker) datamodel.Node {
	return streamBytes{rs}
}

// streamBytes is a boxed reader that complies with datamodel.Node.
type streamBytes struct {
	io.ReadSeeker
}

// -- Node interface methods -->

func (streamBytes) Kind() datamodel.Kind {
	return datamodel.Kind_Bytes
}
func (streamBytes) LookupByString(string) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByString("")
}
func (streamBytes) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByNode(nil)
}
func (streamBytes) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupByIndex(0)
}
func (streamBytes) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return mixins.Bytes{TypeName: "bytes"}.LookupBySegment(seg)
}
func (streamBytes) MapIterator() datamodel.MapIterator {
	return nil
}
func (streamBytes) ListIterator() datamodel.ListIterator {
	return nil
}
func (streamBytes) Length() int64 {
	return -1
}
func (streamBytes) IsAbsent() bool {
	return false
}
func (streamBytes) IsNull() bool {
	return false
}
func (streamBytes) AsBool() (bool, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsBool()
}
func (streamBytes) AsInt() (int64, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsInt()
}
func (streamBytes) AsFloat() (float64, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsFloat()
}
func (streamBytes) AsString() (string, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsString()
}
func (n streamBytes) AsBytes() ([]byte, error) {
	return io.ReadAll(n)
}
func (streamBytes) AsLink() (datamodel.Link, error) {
	return mixins.Bytes{TypeName: "bytes"}.AsLink()
}
func (streamBytes) Prototype() datamodel.NodePrototype {
	return Prototype__Bytes{}
}
func (n streamBytes) AsLargeBytes() (io.ReadSeeker, error) {
	return n.ReadSeeker, nil
}
