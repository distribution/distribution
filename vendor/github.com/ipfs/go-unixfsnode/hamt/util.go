package hamt

// adapted from https://github.com/ipfs/go-unixfs/blob/master/hamt/util.go

import (
	"fmt"

	"math/bits"

	"github.com/Stebalien/go-bitfield"
	"github.com/ipfs/go-unixfsnode/data"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/spaolacci/murmur3"
)

// hashBits is a helper that allows the reading of the 'next n bits' as an integer.
type hashBits struct {
	b        []byte
	consumed int
}

func mkmask(n int) byte {
	return (1 << uint(n)) - 1
}

// Next returns the next 'i' bits of the hashBits value as an integer, or an
// error if there aren't enough bits.
func (hb *hashBits) Next(i int) (int, error) {
	if hb.consumed+i > len(hb.b)*8 {
		return 0, ErrHAMTTooDeep
	}
	return hb.next(i), nil
}

func (hb *hashBits) next(i int) int {
	curbi := hb.consumed / 8
	leftb := 8 - (hb.consumed % 8)

	curb := hb.b[curbi]
	if i == leftb {
		out := int(mkmask(i) & curb)
		hb.consumed += i
		return out
	}
	if i < leftb {
		a := curb & mkmask(leftb) // mask out the high bits we don't want
		b := a & ^mkmask(leftb-i) // mask out the low bits we don't want
		c := b >> uint(leftb-i)   // shift whats left down
		hb.consumed += i
		return int(c)
	}
	out := int(mkmask(leftb) & curb)
	out <<= uint(i - leftb)
	hb.consumed += leftb
	out += hb.next(i - leftb)
	return out

}

func validateHAMTData(nd data.UnixFSData) error {
	if nd.FieldDataType().Int() != data.Data_HAMTShard {
		return data.ErrWrongNodeType{Expected: data.Data_HAMTShard, Actual: nd.FieldDataType().Int()}
	}

	if !nd.FieldHashType().Exists() || uint64(nd.FieldHashType().Must().Int()) != HashMurmur3 {
		return ErrInvalidHashType
	}

	if !nd.FieldData().Exists() {
		return ErrNoDataField
	}

	if !nd.FieldFanout().Exists() {
		return ErrNoFanoutField
	}
	if err := checkLogTwo(int(nd.FieldFanout().Must().Int())); err != nil {
		return err
	}

	return nil
}

func log2Size(nd data.UnixFSData) int {
	return bits.TrailingZeros(uint(nd.FieldFanout().Must().Int()))
}

func maxPadLength(nd data.UnixFSData) int {
	return len(fmt.Sprintf("%X", nd.FieldFanout().Must().Int()-1))
}

func bitField(nd data.UnixFSData) bitfield.Bitfield {
	bf := bitfield.NewBitfield(int(nd.FieldFanout().Must().Int()))
	bf.SetBytes(nd.FieldData().Must().Bytes())
	return bf
}

func checkLogTwo(v int) error {
	if v <= 0 {
		return ErrHAMTSizeInvalid
	}
	lg2 := bits.TrailingZeros(uint(v))
	if 1<<uint(lg2) != v {
		return ErrHAMTSizeInvalid
	}
	return nil
}

func hash(val []byte) []byte {
	h := murmur3.New64()
	h.Write(val)
	return h.Sum(nil)
}

func isValueLink(pbLink dagpb.PBLink, maxPadLen int) (bool, error) {
	if !pbLink.FieldName().Exists() {
		return false, ErrMissingLinkName
	}
	name := pbLink.FieldName().Must().String()
	if len(name) < maxPadLen {
		return false, ErrInvalidLinkName{name}
	}
	if len(name) == maxPadLen {
		return false, nil
	}
	return true, nil
}

func MatchKey(pbLink dagpb.PBLink, key string, maxPadLen int) bool {
	return pbLink.FieldName().Must().String()[maxPadLen:] == key
}
