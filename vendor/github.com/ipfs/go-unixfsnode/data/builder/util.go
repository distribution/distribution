package builder

import (
	"fmt"
	"io"
	"math/bits"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec"
	"github.com/ipld/go-ipld-prime/datamodel"
)

// Common code from go-unixfs/hamt/util.go

// hashBits is a helper for pulling out sections of a hash
type hashBits []byte

func mkmask(n int) byte {
	return (1 << uint(n)) - 1
}

// Slice returns the 'width' bits of the hashBits value as an integer, or an
// error if there aren't enough bits.
func (hb hashBits) Slice(offset, width int) (int, error) {
	if offset+width > len(hb)*8 {
		return 0, fmt.Errorf("sharded directory too deep")
	}
	return hb.slice(offset, width), nil
}

func (hb hashBits) slice(offset, width int) int {
	curbi := offset / 8
	leftb := 8 - (offset % 8)

	curb := hb[curbi]
	if width == leftb {
		out := int(mkmask(width) & curb)
		return out
	} else if width < leftb {
		a := curb & mkmask(leftb)     // mask out the high bits we don't want
		b := a & ^mkmask(leftb-width) // mask out the low bits we don't want
		c := b >> uint(leftb-width)   // shift whats left down
		return int(c)
	} else {
		out := int(mkmask(leftb) & curb)
		out <<= uint(width - leftb)
		out += hb.slice(offset+leftb, width-leftb)
		return out
	}
}

func logtwo(v int) (int, error) {
	if v <= 0 {
		return 0, fmt.Errorf("hamt size should be a power of two")
	}
	lg2 := bits.TrailingZeros(uint(v))
	if 1<<uint(lg2) != v {
		return 0, fmt.Errorf("hamt size should be a power of two")
	}
	return lg2, nil
}

func sizedStore(ls *ipld.LinkSystem, lp datamodel.LinkPrototype, n datamodel.Node) (datamodel.Link, uint64, error) {
	var byteCount int
	lnk, err := wrappedLinkSystem(ls, func(bc int) {
		byteCount = bc
	}).Store(ipld.LinkContext{}, lp, n)
	return lnk, uint64(byteCount), err
}

type byteCounter struct {
	w  io.Writer
	bc int
}

func (bc *byteCounter) Write(p []byte) (int, error) {
	bc.bc += len(p)
	return bc.w.Write(p)
}

func wrappedLinkSystem(ls *ipld.LinkSystem, byteCountCb func(byteCount int)) *ipld.LinkSystem {
	wrappedEncoder := func(encoder codec.Encoder) codec.Encoder {
		return func(node datamodel.Node, writer io.Writer) error {
			bc := byteCounter{w: writer}
			err := encoder(node, &bc)
			if err == nil {
				byteCountCb(bc.bc)
			}
			return err
		}
	}
	wrappedEncoderChooser := func(lp datamodel.LinkPrototype) (codec.Encoder, error) {
		encoder, err := ls.EncoderChooser(lp)
		if err != nil {
			return nil, err
		}
		return wrappedEncoder(encoder), nil
	}
	return &ipld.LinkSystem{
		EncoderChooser:     wrappedEncoderChooser,
		DecoderChooser:     ls.DecoderChooser,
		HasherChooser:      ls.HasherChooser,
		StorageWriteOpener: ls.StorageWriteOpener,
		StorageReadOpener:  ls.StorageReadOpener,
		TrustedStorage:     ls.TrustedStorage,
		NodeReifier:        ls.NodeReifier,
		KnownReifiers:      ls.KnownReifiers,
	}
}
