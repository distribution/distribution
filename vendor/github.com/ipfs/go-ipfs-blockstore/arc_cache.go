package blockstore

import (
	"context"
	"sort"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	metrics "github.com/ipfs/go-metrics-interface"
)

type cacheHave bool
type cacheSize int

type lock struct {
	mu     sync.RWMutex
	refcnt int
}

// arccache wraps a BlockStore with an Adaptive Replacement Cache (ARC) that
// does not store the actual blocks, just metadata about them: existence and
// size. This provides block access-time improvements, allowing
// to short-cut many searches without querying the underlying datastore.
type arccache struct {
	lklk sync.Mutex
	lks  map[string]*lock

	cache *lru.TwoQueueCache

	blockstore Blockstore
	viewer     Viewer

	hits  metrics.Counter
	total metrics.Counter
}

var _ Blockstore = (*arccache)(nil)
var _ Viewer = (*arccache)(nil)

func newARCCachedBS(ctx context.Context, bs Blockstore, lruSize int) (*arccache, error) {
	cache, err := lru.New2Q(lruSize)
	if err != nil {
		return nil, err
	}
	c := &arccache{cache: cache, blockstore: bs, lks: make(map[string]*lock)}
	c.hits = metrics.NewCtx(ctx, "arc.hits_total", "Number of ARC cache hits").Counter()
	c.total = metrics.NewCtx(ctx, "arc_total", "Total number of ARC cache requests").Counter()
	if v, ok := bs.(Viewer); ok {
		c.viewer = v
	}
	return c, nil
}

func (b *arccache) lock(k string, write bool) {
	b.lklk.Lock()
	lk, ok := b.lks[k]
	if !ok {
		lk = new(lock)
		b.lks[k] = lk
	}
	lk.refcnt++
	b.lklk.Unlock()
	if write {
		lk.mu.Lock()
	} else {
		lk.mu.RLock()
	}
}

func (b *arccache) unlock(key string, write bool) {
	b.lklk.Lock()
	lk := b.lks[key]
	lk.refcnt--
	if lk.refcnt == 0 {
		delete(b.lks, key)
	}
	b.lklk.Unlock()
	if write {
		lk.mu.Unlock()
	} else {
		lk.mu.RUnlock()
	}
}

func cacheKey(k cid.Cid) string {
	return string(k.Hash())
}

func (b *arccache) DeleteBlock(ctx context.Context, k cid.Cid) error {
	if !k.Defined() {
		return nil
	}

	key := cacheKey(k)

	if has, _, ok := b.queryCache(key); ok && !has {
		return nil
	}

	b.lock(key, true)
	defer b.unlock(key, true)

	err := b.blockstore.DeleteBlock(ctx, k)
	if err == nil {
		b.cacheHave(key, false)
	} else {
		b.cacheInvalidate(key)
	}
	return err
}

func (b *arccache) Has(ctx context.Context, k cid.Cid) (bool, error) {
	if !k.Defined() {
		return false, nil
	}

	key := cacheKey(k)

	if has, _, ok := b.queryCache(key); ok {
		return has, nil
	}

	b.lock(key, false)
	defer b.unlock(key, false)

	has, err := b.blockstore.Has(ctx, k)
	if err != nil {
		return false, err
	}
	b.cacheHave(key, has)
	return has, nil
}

func (b *arccache) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	if !k.Defined() {
		return -1, ErrNotFound
	}

	key := cacheKey(k)

	if has, blockSize, ok := b.queryCache(key); ok {
		if !has {
			// don't have it, return
			return -1, ErrNotFound
		}
		if blockSize >= 0 {
			// have it and we know the size
			return blockSize, nil
		}
		// we have it but don't know the size, ask the datastore.
	}

	b.lock(key, false)
	defer b.unlock(key, false)

	blockSize, err := b.blockstore.GetSize(ctx, k)
	if err == ErrNotFound {
		b.cacheHave(key, false)
	} else if err == nil {
		b.cacheSize(key, blockSize)
	}
	return blockSize, err
}

func (b *arccache) View(ctx context.Context, k cid.Cid, callback func([]byte) error) error {
	// shortcircuit and fall back to Get if the underlying store
	// doesn't support Viewer.
	if b.viewer == nil {
		blk, err := b.Get(ctx, k)
		if err != nil {
			return err
		}
		return callback(blk.RawData())
	}

	if !k.Defined() {
		return ErrNotFound
	}

	key := cacheKey(k)

	if has, _, ok := b.queryCache(key); ok && !has {
		// short circuit if the cache deterministically tells us the item
		// doesn't exist.
		return ErrNotFound
	}

	b.lock(key, false)
	defer b.unlock(key, false)

	var cberr error
	var size int
	if err := b.viewer.View(ctx, k, func(buf []byte) error {
		size = len(buf)
		cberr = callback(buf)
		return nil
	}); err != nil {
		if err == ErrNotFound {
			b.cacheHave(key, false)
		}
		return err
	}

	b.cacheSize(key, size)

	return cberr
}

func (b *arccache) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	if !k.Defined() {
		return nil, ErrNotFound
	}

	key := cacheKey(k)

	if has, _, ok := b.queryCache(key); ok && !has {
		return nil, ErrNotFound
	}

	b.lock(key, false)
	defer b.unlock(key, false)

	bl, err := b.blockstore.Get(ctx, k)
	if bl == nil && err == ErrNotFound {
		b.cacheHave(key, false)
	} else if bl != nil {
		b.cacheSize(key, len(bl.RawData()))
	}
	return bl, err
}

func (b *arccache) Put(ctx context.Context, bl blocks.Block) error {
	key := cacheKey(bl.Cid())

	if has, _, ok := b.queryCache(key); ok && has {
		return nil
	}

	b.lock(key, true)
	defer b.unlock(key, true)

	err := b.blockstore.Put(ctx, bl)
	if err == nil {
		b.cacheSize(key, len(bl.RawData()))
	} else {
		b.cacheInvalidate(key)
	}
	return err
}

type keyedBlocks struct {
	keys   []string
	blocks []blocks.Block
}

func (b *keyedBlocks) Len() int {
	return len(b.keys)
}

func (b *keyedBlocks) Less(i, j int) bool {
	return b.keys[i] < b.keys[j]
}

func (b *keyedBlocks) Swap(i, j int) {
	b.keys[i], b.keys[j] = b.keys[j], b.keys[i]
	b.blocks[i], b.blocks[j] = b.blocks[j], b.blocks[i]
}

func (b *keyedBlocks) append(key string, blk blocks.Block) {
	b.keys = append(b.keys, key)
	b.blocks = append(b.blocks, blk)
}

func (b *keyedBlocks) isEmpty() bool {
	return len(b.keys) == 0
}

func (b *keyedBlocks) sortAndDedup() {
	if b.isEmpty() {
		return
	}

	sort.Sort(b)

	// https://github.com/golang/go/wiki/SliceTricks#in-place-deduplicate-comparable
	j := 0
	for i := 1; i < len(b.keys); i++ {
		if b.keys[j] == b.keys[i] {
			continue
		}
		j++
		b.keys[j] = b.keys[i]
		b.blocks[j] = b.blocks[i]
	}

	b.keys = b.keys[:j+1]
	b.blocks = b.blocks[:j+1]
}

func newKeyedBlocks(cap int) *keyedBlocks {
	return &keyedBlocks{
		keys:   make([]string, 0, cap),
		blocks: make([]blocks.Block, 0, cap),
	}
}

func (b *arccache) PutMany(ctx context.Context, bs []blocks.Block) error {
	good := newKeyedBlocks(len(bs))
	for _, blk := range bs {
		// call put on block if result is inconclusive or we are sure that
		// the block isn't in storage
		key := cacheKey(blk.Cid())
		if has, _, ok := b.queryCache(key); !ok || (ok && !has) {
			good.append(key, blk)
		}
	}

	if good.isEmpty() {
		return nil
	}

	good.sortAndDedup()

	for _, key := range good.keys {
		b.lock(key, true)
	}

	defer func() {
		for _, key := range good.keys {
			b.unlock(key, true)
		}
	}()

	err := b.blockstore.PutMany(ctx, good.blocks)
	if err != nil {
		return err
	}
	for i, key := range good.keys {
		b.cacheSize(key, len(good.blocks[i].RawData()))
	}

	return nil
}

func (b *arccache) HashOnRead(enabled bool) {
	b.blockstore.HashOnRead(enabled)
}

func (b *arccache) cacheHave(key string, have bool) {
	b.cache.Add(key, cacheHave(have))
}

func (b *arccache) cacheSize(key string, blockSize int) {
	b.cache.Add(key, cacheSize(blockSize))
}

func (b *arccache) cacheInvalidate(key string) {
	b.cache.Remove(key)
}

// queryCache checks if the CID is in the cache. If so, it returns:
//
//  * exists (bool): whether the CID is known to exist or not.
//  * size (int): the size if cached, or -1 if not cached.
//  * ok (bool): whether present in the cache.
//
// When ok is false, the answer in inconclusive and the caller must ignore the
// other two return values. Querying the underying store is necessary.
//
// When ok is true, exists carries the correct answer, and size carries the
// size, if known, or -1 if not.
func (b *arccache) queryCache(k string) (exists bool, size int, ok bool) {
	b.total.Inc()

	h, ok := b.cache.Get(k)
	if ok {
		b.hits.Inc()
		switch h := h.(type) {
		case cacheHave:
			return bool(h), -1, true
		case cacheSize:
			return true, int(h), true
		}
	}
	return false, -1, false
}

func (b *arccache) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return b.blockstore.AllKeysChan(ctx)
}

func (b *arccache) GCLock(ctx context.Context) Unlocker {
	return b.blockstore.(GCBlockstore).GCLock(ctx)
}

func (b *arccache) PinLock(ctx context.Context) Unlocker {
	return b.blockstore.(GCBlockstore).PinLock(ctx)
}

func (b *arccache) GCRequested(ctx context.Context) bool {
	return b.blockstore.(GCBlockstore).GCRequested(ctx)
}
