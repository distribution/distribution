package car

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/index"
	"github.com/ipld/go-car/v2/internal/carv1"
	"github.com/ipld/go-car/v2/internal/loader"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/linking"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
)

// ErrSizeMismatch is returned when a written traversal realizes the written header size does not
// match the actual number of car bytes written.
var ErrSizeMismatch = fmt.Errorf("car-error-sizemismatch")

// ErrOffsetImpossible is returned when specified paddings or offsets of either a wrapped carv1
// or index cannot be satisfied based on the data being written.
var ErrOffsetImpossible = fmt.Errorf("car-error-offsetimpossible")

// MaxTraversalLinks changes the allowed number of links a selector traversal
// can execute before failing.
//
// Note that setting this option may cause an error to be returned from selector
// execution when building a SelectiveCar.
func MaxTraversalLinks(MaxTraversalLinks uint64) Option {
	return func(sco *Options) {
		sco.MaxTraversalLinks = MaxTraversalLinks
	}
}

// NewSelectiveWriter walks through the proposed dag traversal to learn its total size in order to be able to
// stream out a car to a writer in the expected traversal order in one go.
func NewSelectiveWriter(ctx context.Context, ls *ipld.LinkSystem, root cid.Cid, selector ipld.Node, opts ...Option) (Writer, error) {
	cls, cntr := loader.CountingLinkSystem(*ls)

	c1h := carv1.CarHeader{Roots: []cid.Cid{root}, Version: 1}
	headSize, err := carv1.HeaderSize(&c1h)
	if err != nil {
		return nil, err
	}
	if err := traverse(ctx, &cls, root, selector, ApplyOptions(opts...)); err != nil {
		return nil, err
	}
	tc := traversalCar{
		size:     headSize + cntr.Size(),
		ctx:      ctx,
		root:     root,
		selector: selector,
		ls:       ls,
		opts:     ApplyOptions(opts...),
	}
	return &tc, nil
}

// TraverseToFile writes a car file matching a given root and selector to the
// path at `destination` using one read of each block.
func TraverseToFile(ctx context.Context, ls *ipld.LinkSystem, root cid.Cid, selector ipld.Node, destination string, opts ...Option) error {
	tc := traversalCar{
		size:     0,
		ctx:      ctx,
		root:     root,
		selector: selector,
		ls:       ls,
		opts:     ApplyOptions(opts...),
	}

	fp, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer fp.Close()

	_, err = tc.WriteTo(fp)
	if err != nil {
		return err
	}

	// fix header size.
	if _, err = fp.Seek(0, 0); err != nil {
		return err
	}

	tc.size = uint64(tc.size)
	if _, err = tc.WriteV2Header(fp); err != nil {
		return err
	}

	return nil
}

// TraverseV1 walks through the proposed dag traversal and writes a carv1 to the provided io.Writer
func TraverseV1(ctx context.Context, ls *ipld.LinkSystem, root cid.Cid, selector ipld.Node, writer io.Writer, opts ...Option) (uint64, error) {
	opts = append(opts, WithoutIndex())
	tc := traversalCar{
		size:     0,
		ctx:      ctx,
		root:     root,
		selector: selector,
		ls:       ls,
		opts:     ApplyOptions(opts...),
	}

	len, _, err := tc.WriteV1(writer)
	return len, err
}

// Writer is an interface allowing writing a car prepared by PrepareTraversal
type Writer interface {
	io.WriterTo
}

var _ Writer = (*traversalCar)(nil)

type traversalCar struct {
	size     uint64
	ctx      context.Context
	root     cid.Cid
	selector ipld.Node
	ls       *ipld.LinkSystem
	opts     Options
}

func (tc *traversalCar) WriteTo(w io.Writer) (int64, error) {
	n, err := tc.WriteV2Header(w)
	if err != nil {
		return n, err
	}
	v1s, idx, err := tc.WriteV1(w)
	n += int64(v1s)

	if err != nil {
		return n, err
	}

	// index padding, then index
	if tc.opts.IndexCodec != index.CarIndexNone {
		if tc.opts.IndexPadding > 0 {
			buf := make([]byte, tc.opts.IndexPadding)
			pn, err := w.Write(buf)
			n += int64(pn)
			if err != nil {
				return n, err
			}
		}
		in, err := index.WriteTo(idx, w)
		n += int64(in)
		if err != nil {
			return n, err
		}
	}

	return n, err
}

func (tc *traversalCar) WriteV2Header(w io.Writer) (int64, error) {
	n, err := w.Write(Pragma)
	if err != nil {
		return int64(n), err
	}

	h := NewHeader(tc.size)
	if p := tc.opts.DataPadding; p > 0 {
		h = h.WithDataPadding(p)
	}
	if p := tc.opts.IndexPadding; p > 0 {
		h = h.WithIndexPadding(p)
	}
	if tc.opts.IndexCodec == index.CarIndexNone {
		h.IndexOffset = 0
	}
	hn, err := h.WriteTo(w)
	if err != nil {
		return int64(n) + hn, err
	}
	hn += int64(n)

	// We include the initial data padding after the carv2 header
	if h.DataOffset > uint64(hn) {
		// TODO: buffer writes if this needs to be big.
		buf := make([]byte, h.DataOffset-uint64(hn))
		n, err = w.Write(buf)
		hn += int64(n)
		if err != nil {
			return hn, err
		}
	} else if h.DataOffset < uint64(hn) {
		return hn, ErrOffsetImpossible
	}

	return hn, nil
}

func (tc *traversalCar) WriteV1(w io.Writer) (uint64, index.Index, error) {
	// write the v1 header
	c1h := carv1.CarHeader{Roots: []cid.Cid{tc.root}, Version: 1}
	if err := carv1.WriteHeader(&c1h, w); err != nil {
		return 0, nil, err
	}
	v1Size, err := carv1.HeaderSize(&c1h)
	if err != nil {
		return v1Size, nil, err
	}

	// write the block.
	wls, writer := loader.TeeingLinkSystem(*tc.ls, w, v1Size, tc.opts.IndexCodec)
	err = traverse(tc.ctx, &wls, tc.root, tc.selector, tc.opts)
	v1Size = writer.Size()
	if err != nil {
		return v1Size, nil, err
	}
	if tc.size != 0 && tc.size != v1Size {
		return v1Size, nil, ErrSizeMismatch
	}
	tc.size = v1Size

	if tc.opts.IndexCodec == index.CarIndexNone {
		return v1Size, nil, nil
	}
	idx, err := writer.Index()
	return v1Size, idx, err
}

func traverse(ctx context.Context, ls *ipld.LinkSystem, root cid.Cid, s ipld.Node, opts Options) error {
	sel, err := selector.CompileSelector(s)
	if err != nil {
		return err
	}

	progress := traversal.Progress{
		Cfg: &traversal.Config{
			Ctx:        ctx,
			LinkSystem: *ls,
			LinkTargetNodePrototypeChooser: func(_ ipld.Link, _ linking.LinkContext) (ipld.NodePrototype, error) {
				return basicnode.Prototype.Any, nil
			},
			LinkVisitOnlyOnce: !opts.BlockstoreAllowDuplicatePuts,
		},
	}
	if opts.MaxTraversalLinks < math.MaxInt64 {
		progress.Budget = &traversal.Budget{
			NodeBudget: math.MaxInt64,
			LinkBudget: int64(opts.MaxTraversalLinks),
		}
	}

	lnk := cidlink.Link{Cid: root}
	ls.TrustedStorage = true
	rootNode, err := ls.Load(ipld.LinkContext{}, lnk, basicnode.Prototype.Any)
	if err != nil {
		return fmt.Errorf("root blk load failed: %s", err)
	}
	err = progress.WalkMatching(rootNode, sel, func(_ traversal.Progress, _ ipld.Node) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk failed: %s", err)
	}
	return nil
}
