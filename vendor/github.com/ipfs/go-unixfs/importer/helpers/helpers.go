package helpers

import (
	"fmt"
)

// BlockSizeLimit specifies the maximum size an imported block can have.
var BlockSizeLimit = 1048576 // 1 MB

// rough estimates on expected sizes
var roughLinkBlockSize = 1 << 13 // 8KB
var roughLinkSize = 34 + 8 + 5   // sha256 multihash + size + no name + protobuf framing

// DefaultLinksPerBlock governs how the importer decides how many links there
// will be per block. This calculation is based on expected distributions of:
//  * the expected distribution of block sizes
//  * the expected distribution of link sizes
//  * desired access speed
// For now, we use:
//
//   var roughLinkBlockSize = 1 << 13 // 8KB
//   var roughLinkSize = 34 + 8 + 5   // sha256 multihash + size + no name
//                                    // + protobuf framing
//   var DefaultLinksPerBlock = (roughLinkBlockSize / roughLinkSize)
//                            = ( 8192 / 47 )
//                            = (approximately) 174
var DefaultLinksPerBlock = roughLinkBlockSize / roughLinkSize

// ErrSizeLimitExceeded signals that a block is larger than BlockSizeLimit.
var ErrSizeLimitExceeded = fmt.Errorf("object size limit exceeded")
