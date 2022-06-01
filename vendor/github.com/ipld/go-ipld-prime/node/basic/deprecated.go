// This is a transitional package: please move your references to `node/basicnode`.
// The new package is identical: we've renamed the import path only.
//
// All content in this package is a thin wrapper around `node/basicnode`.
// Please update at your earliest convenience.
//
// This package will eventually be removed.
package basicnode

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/linking"
	"github.com/ipld/go-ipld-prime/node/basicnode"
)

var Prototype = basicnode.Prototype

func Chooser(_ datamodel.Link, _ linking.LinkContext) (datamodel.NodePrototype, error) {
	return basicnode.Chooser(nil, linking.LinkContext{})
}
func NewBool(value bool) datamodel.Node           { return basicnode.NewBool(value) }
func NewBytes(value []byte) datamodel.Node        { return basicnode.NewBytes(value) }
func NewFloat(value float64) datamodel.Node       { return basicnode.NewFloat(value) }
func NewInt(value int64) datamodel.Node           { return basicnode.NewInt(value) }
func NewLink(value datamodel.Link) datamodel.Node { return basicnode.NewLink(value) }
func NewString(value string) datamodel.Node       { return basicnode.NewString(value) }

type Prototype__Any = basicnode.Prototype__Any
type Prototype__Bool = basicnode.Prototype__Bool
type Prototype__Bytes = basicnode.Prototype__Bytes
type Prototype__Float = basicnode.Prototype__Float
type Prototype__Int = basicnode.Prototype__Int
type Prototype__Link = basicnode.Prototype__Link
type Prototype__List = basicnode.Prototype__List
type Prototype__Map = basicnode.Prototype__Map
type Prototype__String = basicnode.Prototype__String
