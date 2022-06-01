package blockstore

import (
	"context"
	"io"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// idstore wraps a BlockStore to add support for identity hashes
type idstore struct {
	bs     Blockstore
	viewer Viewer
}

var _ Blockstore = (*idstore)(nil)
var _ Viewer = (*idstore)(nil)
var _ io.Closer = (*idstore)(nil)

func NewIdStore(bs Blockstore) Blockstore {
	ids := &idstore{bs: bs}
	if v, ok := bs.(Viewer); ok {
		ids.viewer = v
	}
	return ids
}

func extractContents(k cid.Cid) (bool, []byte) {
	// Pre-check by calling Prefix(), this much faster than extracting the hash.
	if k.Prefix().MhType != mh.IDENTITY {
		return false, nil
	}

	dmh, err := mh.Decode(k.Hash())
	if err != nil || dmh.Code != mh.IDENTITY {
		return false, nil
	}
	return true, dmh.Digest
}

func (b *idstore) DeleteBlock(ctx context.Context, k cid.Cid) error {
	isId, _ := extractContents(k)
	if isId {
		return nil
	}
	return b.bs.DeleteBlock(ctx, k)
}

func (b *idstore) Has(ctx context.Context, k cid.Cid) (bool, error) {
	isId, _ := extractContents(k)
	if isId {
		return true, nil
	}
	return b.bs.Has(ctx, k)
}

func (b *idstore) View(ctx context.Context, k cid.Cid, callback func([]byte) error) error {
	if b.viewer == nil {
		blk, err := b.Get(ctx, k)
		if err != nil {
			return err
		}
		return callback(blk.RawData())
	}
	isId, bdata := extractContents(k)
	if isId {
		return callback(bdata)
	}
	return b.viewer.View(ctx, k, callback)
}

func (b *idstore) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	isId, bdata := extractContents(k)
	if isId {
		return len(bdata), nil
	}
	return b.bs.GetSize(ctx, k)
}

func (b *idstore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	isId, bdata := extractContents(k)
	if isId {
		return blocks.NewBlockWithCid(bdata, k)
	}
	return b.bs.Get(ctx, k)
}

func (b *idstore) Put(ctx context.Context, bl blocks.Block) error {
	isId, _ := extractContents(bl.Cid())
	if isId {
		return nil
	}
	return b.bs.Put(ctx, bl)
}

func (b *idstore) PutMany(ctx context.Context, bs []blocks.Block) error {
	toPut := make([]blocks.Block, 0, len(bs))
	for _, bl := range bs {
		isId, _ := extractContents(bl.Cid())
		if isId {
			continue
		}
		toPut = append(toPut, bl)
	}
	return b.bs.PutMany(ctx, toPut)
}

func (b *idstore) HashOnRead(enabled bool) {
	b.bs.HashOnRead(enabled)
}

func (b *idstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.bs.AllKeysChan(ctx)
}

func (b *idstore) Close() error {
	if c, ok := b.bs.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
