package bitfield

// NOTE: Don't bother replacing the divisions/modulo with shifts/ands, go is smart.

import (
	"math/bits"
)

// NewBitfield creates a new fixed-sized Bitfield (allocated up-front).
//
// Panics if size is not a multiple of 8.
func NewBitfield(size int) Bitfield {
	if size%8 != 0 {
		panic("Bitfield size must be a multiple of 8")
	}
	return make([]byte, size/8)
}

// FromBytes constructs a new bitfield from a serialized bitfield.
func FromBytes(size int, bits []byte) Bitfield {
	bf := NewBitfield(size)
	start := len(bf) - len(bits)
	if start < 0 {
		panic("bitfield too small")
	}
	copy(bf[start:], bits)
	return bf
}

func (bf Bitfield) offset(i int) (uint, uint8) {
	return uint(len(bf)) - (uint(i) / 8) - 1, uint8(i) % 8
}

// Bitfield is, well, a bitfield.
type Bitfield []byte

// Bytes returns the Bitfield as a byte string.
//
// This function *does not* copy.
func (bf Bitfield) Bytes() []byte {
	for i, b := range bf {
		if b != 0 {
			return bf[i:]
		}
	}
	return nil
}

// Bit returns the ith bit.
//
// Panics if the bit is out of bounds.
func (bf Bitfield) Bit(i int) bool {
	idx, off := bf.offset(i)
	return (bf[idx]>>off)&0x1 != 0
}

// SetBit sets the ith bit.
//
// Panics if the bit is out of bounds.
func (bf Bitfield) SetBit(i int) {
	idx, off := bf.offset(i)
	bf[idx] |= 1 << off
}

// UnsetBit unsets the ith bit.
//
// Panics if the bit is out of bounds.
func (bf Bitfield) UnsetBit(i int) {
	idx, off := bf.offset(i)
	bf[idx] &= 0xFF ^ (1 << off)
}

// SetBytes sets the bits to the given byte array.
//
// Panics if 'b' is larger than the bitfield.
func (bf Bitfield) SetBytes(b []byte) {
	start := len(bf) - len(b)
	if start < 0 {
		panic("bitfield too small")
	}
	for i := range bf[:start] {
		bf[i] = 0
	}
	copy(bf[start:], b)
}

// Ones returns the number of bits set.
func (bf Bitfield) Ones() int {
	cnt := 0
	for _, b := range bf {
		cnt += bits.OnesCount8(b)
	}
	return cnt
}

// OnesBefore returns the number of bits set *before* this bit.
func (bf Bitfield) OnesBefore(i int) int {
	idx, off := bf.offset(i)
	cnt := bits.OnesCount8(bf[idx] << (8 - off))
	for _, b := range bf[idx+1:] {
		cnt += bits.OnesCount8(b)
	}
	return cnt
}

// OnesAfter returns the number of bits set *after* this bit.
func (bf Bitfield) OnesAfter(i int) int {
	idx, off := bf.offset(i)
	cnt := bits.OnesCount8(bf[idx] >> off)
	for _, b := range bf[:idx] {
		cnt += bits.OnesCount8(b)
	}
	return cnt
}
