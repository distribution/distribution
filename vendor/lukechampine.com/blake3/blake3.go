// Package blake3 implements the BLAKE3 cryptographic hash function.
package blake3 // import "lukechampine.com/blake3"

import (
	"encoding/binary"
	"errors"
	"hash"
	"io"
	"math"
	"math/bits"
)

const (
	flagChunkStart = 1 << iota
	flagChunkEnd
	flagParent
	flagRoot
	flagKeyedHash
	flagDeriveKeyContext
	flagDeriveKeyMaterial

	blockSize = 64
	chunkSize = 1024

	maxSIMD = 16 // AVX-512 vectors can store 16 words
)

var iv = [8]uint32{
	0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
	0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
}

// A node represents a chunk or parent in the BLAKE3 Merkle tree.
type node struct {
	cv       [8]uint32 // chaining value from previous node
	block    [16]uint32
	counter  uint64
	blockLen uint32
	flags    uint32
}

// parentNode returns a node that incorporates the chaining values of two child
// nodes.
func parentNode(left, right [8]uint32, key [8]uint32, flags uint32) node {
	n := node{
		cv:       key,
		counter:  0,         // counter is reset for parents
		blockLen: blockSize, // block is full
		flags:    flags | flagParent,
	}
	copy(n.block[:8], left[:])
	copy(n.block[8:], right[:])
	return n
}

// Hasher implements hash.Hash.
type Hasher struct {
	key   [8]uint32
	flags uint32
	size  int // output size, for Sum

	// log(n) set of Merkle subtree roots, at most one per height.
	stack   [50][8]uint32 // 2^50 * maxSIMD * chunkSize = 2^64
	counter uint64        // number of buffers hashed; also serves as a bit vector indicating which stack elems are occupied

	buf    [maxSIMD * chunkSize]byte
	buflen int
}

func (h *Hasher) hasSubtreeAtHeight(i int) bool {
	return h.counter&(1<<i) != 0
}

func (h *Hasher) pushSubtree(cv [8]uint32) {
	// seek to first open stack slot, merging subtrees as we go
	i := 0
	for h.hasSubtreeAtHeight(i) {
		cv = chainingValue(parentNode(h.stack[i], cv, h.key, h.flags))
		i++
	}
	h.stack[i] = cv
	h.counter++
}

// rootNode computes the root of the Merkle tree. It does not modify the
// stack.
func (h *Hasher) rootNode() node {
	n := compressBuffer(&h.buf, h.buflen, &h.key, h.counter*maxSIMD, h.flags)
	for i := bits.TrailingZeros64(h.counter); i < bits.Len64(h.counter); i++ {
		if h.hasSubtreeAtHeight(i) {
			n = parentNode(h.stack[i], chainingValue(n), h.key, h.flags)
		}
	}
	n.flags |= flagRoot
	return n
}

// Write implements hash.Hash.
func (h *Hasher) Write(p []byte) (int, error) {
	lenp := len(p)
	for len(p) > 0 {
		if h.buflen == len(h.buf) {
			n := compressBuffer(&h.buf, h.buflen, &h.key, h.counter*maxSIMD, h.flags)
			h.pushSubtree(chainingValue(n))
			h.buflen = 0
		}
		n := copy(h.buf[h.buflen:], p)
		h.buflen += n
		p = p[n:]
	}
	return lenp, nil
}

// Sum implements hash.Hash.
func (h *Hasher) Sum(b []byte) (sum []byte) {
	// We need to append h.Size() bytes to b. Reuse b's capacity if possible;
	// otherwise, allocate a new slice.
	if total := len(b) + h.Size(); cap(b) >= total {
		sum = b[:total]
	} else {
		sum = make([]byte, total)
		copy(sum, b)
	}
	// Read into the appended portion of sum. Use a low-latency-low-throughput
	// path for small digests (requiring a single compression), and a
	// high-latency-high-throughput path for large digests.
	if dst := sum[len(b):]; len(dst) <= 64 {
		var out [64]byte
		wordsToBytes(compressNode(h.rootNode()), &out)
		copy(dst, out[:])
	} else {
		h.XOF().Read(dst)
	}
	return
}

// Reset implements hash.Hash.
func (h *Hasher) Reset() {
	h.counter = 0
	h.buflen = 0
}

// BlockSize implements hash.Hash.
func (h *Hasher) BlockSize() int { return 64 }

// Size implements hash.Hash.
func (h *Hasher) Size() int { return h.size }

// XOF returns an OutputReader initialized with the current hash state.
func (h *Hasher) XOF() *OutputReader {
	return &OutputReader{
		n: h.rootNode(),
	}
}

func newHasher(key [8]uint32, flags uint32, size int) *Hasher {
	return &Hasher{
		key:   key,
		flags: flags,
		size:  size,
	}
}

// New returns a Hasher for the specified size and key. If key is nil, the hash
// is unkeyed. Otherwise, len(key) must be 32.
func New(size int, key []byte) *Hasher {
	if key == nil {
		return newHasher(iv, 0, size)
	}
	var keyWords [8]uint32
	for i := range keyWords {
		keyWords[i] = binary.LittleEndian.Uint32(key[i*4:])
	}
	return newHasher(keyWords, flagKeyedHash, size)
}

// Sum256 and Sum512 always use the same hasher state, so we can save some time
// when hashing small inputs by constructing the hasher ahead of time.
var defaultHasher = New(0, nil)

// Sum256 returns the unkeyed BLAKE3 hash of b, truncated to 256 bits.
func Sum256(b []byte) (out [32]byte) {
	out512 := Sum512(b)
	copy(out[:], out512[:])
	return
}

// Sum512 returns the unkeyed BLAKE3 hash of b, truncated to 512 bits.
func Sum512(b []byte) (out [64]byte) {
	var n node
	if len(b) <= blockSize {
		hashBlock(&out, b)
		return
	} else if len(b) <= chunkSize {
		n = compressChunk(b, &iv, 0, 0)
		n.flags |= flagRoot
	} else {
		h := *defaultHasher
		h.Write(b)
		n = h.rootNode()
	}
	wordsToBytes(compressNode(n), &out)
	return
}

// DeriveKey derives a subkey from ctx and srcKey. ctx should be hardcoded,
// globally unique, and application-specific. A good format for ctx strings is:
//
//    [application] [commit timestamp] [purpose]
//
// e.g.:
//
//    example.com 2019-12-25 16:18:03 session tokens v1
//
// The purpose of these requirements is to ensure that an attacker cannot trick
// two different applications into using the same context string.
func DeriveKey(subKey []byte, ctx string, srcKey []byte) {
	// construct the derivation Hasher
	const derivationIVLen = 32
	h := newHasher(iv, flagDeriveKeyContext, 32)
	h.Write([]byte(ctx))
	derivationIV := h.Sum(make([]byte, 0, derivationIVLen))
	var ivWords [8]uint32
	for i := range ivWords {
		ivWords[i] = binary.LittleEndian.Uint32(derivationIV[i*4:])
	}
	h = newHasher(ivWords, flagDeriveKeyMaterial, 0)
	// derive the subKey
	h.Write(srcKey)
	h.XOF().Read(subKey)
}

// An OutputReader produces an seekable stream of 2^64 - 1 pseudorandom output
// bytes.
type OutputReader struct {
	n   node
	buf [maxSIMD * blockSize]byte
	off uint64
}

// Read implements io.Reader. Callers may assume that Read returns len(p), nil
// unless the read would extend beyond the end of the stream.
func (or *OutputReader) Read(p []byte) (int, error) {
	if or.off == math.MaxUint64 {
		return 0, io.EOF
	} else if rem := math.MaxUint64 - or.off; uint64(len(p)) > rem {
		p = p[:rem]
	}
	lenp := len(p)
	for len(p) > 0 {
		if or.off%(maxSIMD*blockSize) == 0 {
			or.n.counter = or.off / blockSize
			compressBlocks(&or.buf, or.n)
		}
		n := copy(p, or.buf[or.off%(maxSIMD*blockSize):])
		p = p[n:]
		or.off += uint64(n)
	}
	return lenp, nil
}

// Seek implements io.Seeker.
func (or *OutputReader) Seek(offset int64, whence int) (int64, error) {
	off := or.off
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, errors.New("seek position cannot be negative")
		}
		off = uint64(offset)
	case io.SeekCurrent:
		if offset < 0 {
			if uint64(-offset) > off {
				return 0, errors.New("seek position cannot be negative")
			}
			off -= uint64(-offset)
		} else {
			off += uint64(offset)
		}
	case io.SeekEnd:
		off = uint64(offset) - 1
	default:
		panic("invalid whence")
	}
	or.off = off
	or.n.counter = uint64(off) / blockSize
	if or.off%(maxSIMD*blockSize) != 0 {
		compressBlocks(&or.buf, or.n)
	}
	// NOTE: or.off >= 2^63 will result in a negative return value.
	// Nothing we can do about this.
	return int64(or.off), nil
}

// ensure that Hasher implements hash.Hash
var _ hash.Hash = (*Hasher)(nil)
