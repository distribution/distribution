package car

import (
	"encoding/binary"
	"io"
)

const (
	// PragmaSize is the size of the CARv2 pragma in bytes.
	PragmaSize = 11
	// HeaderSize is the fixed size of CARv2 header in number of bytes.
	HeaderSize = 40
	// CharacteristicsSize is the fixed size of Characteristics bitfield within CARv2 header in number of bytes.
	CharacteristicsSize = 16
)

// The pragma of a CARv2, containing the version number.
// This is a valid CARv1 header, with version number of 2 and no root CIDs.
var Pragma = []byte{
	0x0a,                                     // unit(10)
	0xa1,                                     // map(1)
	0x67,                                     // string(7)
	0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, // "version"
	0x02, // uint(2)
}

type (
	// Header represents the CARv2 header/pragma.
	Header struct {
		// 128-bit characteristics of this CARv2 file, such as order, deduplication, etc. Reserved for future use.
		Characteristics Characteristics
		// The byte-offset from the beginning of the CARv2 to the first byte of the CARv1 data payload.
		DataOffset uint64
		// The byte-length of the CARv1 data payload.
		DataSize uint64
		// The byte-offset from the beginning of the CARv2 to the first byte of the index payload. This value may be 0 to indicate the absence of index data.
		IndexOffset uint64
	}
	// Characteristics is a bitfield placeholder for capturing the characteristics of a CARv2 such as order and determinism.
	Characteristics struct {
		Hi uint64
		Lo uint64
	}
)

// fullyIndexedCharPos is the position of Characteristics.Hi bit that specifies whether the index is a catalog af all CIDs or not.
const fullyIndexedCharPos = 7 // left-most bit

// WriteTo writes this characteristics to the given w.
func (c Characteristics) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[:8], c.Hi)
	binary.LittleEndian.PutUint64(buf[8:], c.Lo)
	written, err := w.Write(buf)
	return int64(written), err
}

func (c *Characteristics) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, CharacteristicsSize)
	read, err := io.ReadFull(r, buf)
	n := int64(read)
	if err != nil {
		return n, err
	}
	c.Hi = binary.LittleEndian.Uint64(buf[:8])
	c.Lo = binary.LittleEndian.Uint64(buf[8:])
	return n, nil
}

// IsFullyIndexed specifies whether the index of CARv2 represents a catalog of all CID segments.
// See StoreIdentityCIDs
func (c *Characteristics) IsFullyIndexed() bool {
	return isBitSet(c.Hi, fullyIndexedCharPos)
}

// SetFullyIndexed sets whether of CARv2 represents a catalog of all CID segments.
func (c *Characteristics) SetFullyIndexed(b bool) {
	if b {
		c.Hi = setBit(c.Hi, fullyIndexedCharPos)
	} else {
		c.Hi = unsetBit(c.Hi, fullyIndexedCharPos)
	}
}

func setBit(n uint64, pos uint) uint64 {
	n |= 1 << pos
	return n
}

func unsetBit(n uint64, pos uint) uint64 {
	mask := uint64(^(1 << pos))
	n &= mask
	return n
}

func isBitSet(n uint64, pos uint) bool {
	bit := n & (1 << pos)
	return bit > 0
}

// NewHeader instantiates a new CARv2 header, given the data size.
func NewHeader(dataSize uint64) Header {
	header := Header{
		DataSize: dataSize,
	}
	header.DataOffset = PragmaSize + HeaderSize
	header.IndexOffset = header.DataOffset + dataSize
	return header
}

// WithIndexPadding sets the index offset from the beginning of the file for this header and returns
// the header for convenient chained calls.
// The index offset is calculated as the sum of PragmaSize, HeaderSize,
// Header.DataSize, and the given padding.
func (h Header) WithIndexPadding(padding uint64) Header {
	h.IndexOffset = h.IndexOffset + padding
	return h
}

// WithDataPadding sets the data payload byte-offset from the beginning of the file for this header
// and returns the header for convenient chained calls.
// The Data offset is calculated as the sum of PragmaSize, HeaderSize and the given padding.
// The call to this function also shifts the Header.IndexOffset forward by the given padding.
func (h Header) WithDataPadding(padding uint64) Header {
	h.DataOffset = PragmaSize + HeaderSize + padding
	h.IndexOffset = h.IndexOffset + padding
	return h
}

func (h Header) WithDataSize(size uint64) Header {
	h.DataSize = size
	h.IndexOffset = size + h.IndexOffset
	return h
}

// HasIndex indicates whether the index is present.
func (h Header) HasIndex() bool {
	return h.IndexOffset != 0
}

// WriteTo serializes this header as bytes and writes them using the given io.Writer.
func (h Header) WriteTo(w io.Writer) (n int64, err error) {
	wn, err := h.Characteristics.WriteTo(w)
	n += wn
	if err != nil {
		return
	}
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint64(buf[:8], h.DataOffset)
	binary.LittleEndian.PutUint64(buf[8:16], h.DataSize)
	binary.LittleEndian.PutUint64(buf[16:], h.IndexOffset)
	written, err := w.Write(buf)
	n += int64(written)
	return n, err
}

// ReadFrom populates fields of this header from the given r.
func (h *Header) ReadFrom(r io.Reader) (int64, error) {
	n, err := h.Characteristics.ReadFrom(r)
	if err != nil {
		return n, err
	}
	buf := make([]byte, 24)
	read, err := io.ReadFull(r, buf)
	n += int64(read)
	if err != nil {
		return n, err
	}
	h.DataOffset = binary.LittleEndian.Uint64(buf[:8])
	h.DataSize = binary.LittleEndian.Uint64(buf[8:16])
	h.IndexOffset = binary.LittleEndian.Uint64(buf[16:])
	return n, nil
}
