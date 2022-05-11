package ipld

import (
	"github.com/ipld/go-ipld-prime/datamodel"
)

// DeepEqual reports whether x and y are "deeply equal" as IPLD nodes.
// This is similar to reflect.DeepEqual, but based around the Node interface.
//
// This is exactly equivalent to the datamodel.DeepEqual function.
func DeepEqual(x, y Node) bool {
	return datamodel.DeepEqual(x, y)
}
