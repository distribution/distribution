package carv1

import (
	"context"
	"fmt"
	"io"

	"github.com/ipld/go-car/v2/internal/carv1/util"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
)

func init() {
	cbor.RegisterCborType(CarHeader{})
}

type Store interface {
	Put(context.Context, blocks.Block) error
}

type ReadStore interface {
	Get(context.Context, cid.Cid) (blocks.Block, error)
}

type CarHeader struct {
	Roots   []cid.Cid
	Version uint64
}

type carWriter struct {
	ds format.NodeGetter
	w  io.Writer
}

func WriteCar(ctx context.Context, ds format.NodeGetter, roots []cid.Cid, w io.Writer) error {
	h := &CarHeader{
		Roots:   roots,
		Version: 1,
	}

	if err := WriteHeader(h, w); err != nil {
		return fmt.Errorf("failed to write car header: %s", err)
	}

	cw := &carWriter{ds: ds, w: w}
	seen := cid.NewSet()
	for _, r := range roots {
		if err := merkledag.Walk(ctx, cw.enumGetLinks, r, seen.Visit); err != nil {
			return err
		}
	}
	return nil
}

func ReadHeader(r io.Reader) (*CarHeader, error) {
	hb, err := util.LdRead(r, false)
	if err != nil {
		return nil, err
	}

	var ch CarHeader
	if err := cbor.DecodeInto(hb, &ch); err != nil {
		return nil, fmt.Errorf("invalid header: %v", err)
	}

	return &ch, nil
}

func WriteHeader(h *CarHeader, w io.Writer) error {
	hb, err := cbor.DumpObject(h)
	if err != nil {
		return err
	}

	return util.LdWrite(w, hb)
}

func HeaderSize(h *CarHeader) (uint64, error) {
	hb, err := cbor.DumpObject(h)
	if err != nil {
		return 0, err
	}

	return util.LdSize(hb), nil
}

func (cw *carWriter) enumGetLinks(ctx context.Context, c cid.Cid) ([]*format.Link, error) {
	nd, err := cw.ds.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	if err := cw.writeNode(ctx, nd); err != nil {
		return nil, err
	}

	return nd.Links(), nil
}

func (cw *carWriter) writeNode(ctx context.Context, nd format.Node) error {
	return util.LdWrite(cw.w, nd.Cid().Bytes(), nd.RawData())
}

type CarReader struct {
	r            io.Reader
	Header       *CarHeader
	zeroLenAsEOF bool
}

func NewCarReaderWithZeroLengthSectionAsEOF(r io.Reader) (*CarReader, error) {
	return newCarReader(r, true)
}

func NewCarReader(r io.Reader) (*CarReader, error) {
	return newCarReader(r, false)
}

func newCarReader(r io.Reader, zeroLenAsEOF bool) (*CarReader, error) {
	ch, err := ReadHeader(r)
	if err != nil {
		return nil, err
	}

	if ch.Version != 1 {
		return nil, fmt.Errorf("invalid car version: %d", ch.Version)
	}

	if len(ch.Roots) == 0 {
		return nil, fmt.Errorf("empty car, no roots")
	}

	return &CarReader{
		r:            r,
		Header:       ch,
		zeroLenAsEOF: zeroLenAsEOF,
	}, nil
}

func (cr *CarReader) Next() (blocks.Block, error) {
	c, data, err := util.ReadNode(cr.r, cr.zeroLenAsEOF)
	if err != nil {
		return nil, err
	}

	hashed, err := c.Prefix().Sum(data)
	if err != nil {
		return nil, err
	}

	if !hashed.Equals(c) {
		return nil, fmt.Errorf("mismatch in content integrity, name: %s, data: %s", c, hashed)
	}

	return blocks.NewBlockWithCid(data, c)
}

type batchStore interface {
	PutMany(context.Context, []blocks.Block) error
}

func LoadCar(s Store, r io.Reader) (*CarHeader, error) {
	ctx := context.TODO()
	cr, err := NewCarReader(r)
	if err != nil {
		return nil, err
	}

	if bs, ok := s.(batchStore); ok {
		return loadCarFast(ctx, bs, cr)
	}

	return loadCarSlow(ctx, s, cr)
}

func loadCarFast(ctx context.Context, s batchStore, cr *CarReader) (*CarHeader, error) {
	var buf []blocks.Block
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				if len(buf) > 0 {
					if err := s.PutMany(ctx, buf); err != nil {
						return nil, err
					}
				}
				return cr.Header, nil
			}
			return nil, err
		}

		buf = append(buf, blk)

		if len(buf) > 1000 {
			if err := s.PutMany(ctx, buf); err != nil {
				return nil, err
			}
			buf = buf[:0]
		}
	}
}

func loadCarSlow(ctx context.Context, s Store, cr *CarReader) (*CarHeader, error) {
	for {
		blk, err := cr.Next()
		if err != nil {
			if err == io.EOF {
				return cr.Header, nil
			}
			return nil, err
		}

		if err := s.Put(ctx, blk); err != nil {
			return nil, err
		}
	}
}

// Matches checks whether two headers match.
// Two headers are considered matching if:
//   1. They have the same version number, and
//   2. They contain the same root CIDs in any order.
// Note, this function explicitly ignores the order of roots.
// If order of roots matter use reflect.DeepEqual instead.
func (h CarHeader) Matches(other CarHeader) bool {
	if h.Version != other.Version {
		return false
	}
	thisLen := len(h.Roots)
	if thisLen != len(other.Roots) {
		return false
	}
	// Headers with a single root are popular.
	// Implement a fast execution path for popular cases.
	if thisLen == 1 {
		return h.Roots[0].Equals(other.Roots[0])
	}

	// Check other contains all roots.
	// TODO: should this be optimised for cases where the number of roots are large since it has O(N^2) complexity?
	for _, r := range h.Roots {
		if !other.containsRoot(r) {
			return false
		}
	}
	return true
}

func (h *CarHeader) containsRoot(root cid.Cid) bool {
	for _, r := range h.Roots {
		if r.Equals(root) {
			return true
		}
	}
	return false
}
