package multihash

import "errors"

// ErrSumNotSupported is returned when the Sum function code is not implemented
var ErrSumNotSupported = errors.New("no such hash registered")

// constants
const (
	IDENTITY      = 0x00
	SHA1          = 0x11
	SHA2_224      = 0x1013
	SHA2_256      = 0x12
	SHA2_384      = 0x20
	SHA2_512      = 0x13
	SHA2_512_224  = 0x1014
	SHA2_512_256  = 0x1015
	SHA3_224      = 0x17
	SHA3_256      = 0x16
	SHA3_384      = 0x15
	SHA3_512      = 0x14
	KECCAK_224    = 0x1A
	KECCAK_256    = 0x1B
	KECCAK_384    = 0x1C
	KECCAK_512    = 0x1D
	BLAKE3        = 0x1E
	SHAKE_128     = 0x18
	SHAKE_256     = 0x19
	MURMUR3X64_64 = 0x22
	MD5           = 0xd5
	DBL_SHA2_256  = 0x56
)
