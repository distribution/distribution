package hamt

import (
	"context"
	"fmt"

	"github.com/Stebalien/go-bitfield"
	"github.com/ipfs/go-unixfsnode/data"
	"github.com/ipfs/go-unixfsnode/iter"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/schema"
)

const (
	// HashMurmur3 is the multiformats identifier for Murmur3
	HashMurmur3 uint64 = 0x22
)

var _ ipld.Node = UnixFSHAMTShard(nil)
var _ schema.TypedNode = UnixFSHAMTShard(nil)
var _ ipld.ADL = UnixFSHAMTShard(nil)

// UnixFSHAMTShared is an IPLD Prime Node that provides a read interface
// to a UnixFS HAMT
type UnixFSHAMTShard = *_UnixFSHAMTShard

type _UnixFSHAMTShard struct {
	ctx          context.Context
	_substrate   dagpb.PBNode
	data         data.UnixFSData
	lsys         *ipld.LinkSystem
	bitfield     bitfield.Bitfield
	shardCache   map[ipld.Link]*_UnixFSHAMTShard
	cachedLength int64
}

// NewUnixFSHAMTShard attempts to construct a UnixFSHAMTShard node from the base protobuf node plus
// a decoded UnixFSData structure
func NewUnixFSHAMTShard(ctx context.Context, substrate dagpb.PBNode, data data.UnixFSData, lsys *ipld.LinkSystem) (ipld.Node, error) {
	if err := validateHAMTData(data); err != nil {
		return nil, err
	}
	shardCache := make(map[ipld.Link]*_UnixFSHAMTShard, substrate.FieldLinks().Length())
	bf := bitField(data)
	return &_UnixFSHAMTShard{
		ctx:          ctx,
		_substrate:   substrate,
		data:         data,
		lsys:         lsys,
		shardCache:   shardCache,
		bitfield:     bf,
		cachedLength: -1,
	}, nil
}

func (n UnixFSHAMTShard) Substrate() ipld.Node {
	return n._substrate
}

func (n UnixFSHAMTShard) Kind() ipld.Kind {
	return n._substrate.Kind()
}

// LookupByString looks for the key in the list of links with a matching name
func (n *_UnixFSHAMTShard) LookupByString(key string) (ipld.Node, error) {
	hv := &hashBits{b: hash([]byte(key))}
	return n.lookup(key, hv)
}

func (n UnixFSHAMTShard) lookup(key string, hv *hashBits) (dagpb.Link, error) {
	log2 := log2Size(n.data)
	maxPadLen := maxPadLength(n.data)
	childIndex, err := hv.Next(log2)
	if err != nil {
		return nil, err
	}

	if n.hasChild(childIndex) {
		pbLink, err := n.getChildLink(childIndex)
		if err != nil {
			return nil, err
		}
		isValue, err := isValueLink(pbLink, maxPadLen)
		if err != nil {
			return nil, err
		}
		if isValue {
			if MatchKey(pbLink, key, maxPadLen) {
				return pbLink.FieldHash(), nil
			}
		} else {
			childNd, err := n.loadChild(pbLink)
			if err != nil {
				return nil, err
			}
			return childNd.lookup(key, hv)
		}
	}
	return nil, schema.ErrNoSuchField{Type: nil /*TODO*/, Field: ipld.PathSegmentOfString(key)}
}

// AttemptHAMTShardFromNode attempts to read a HAMT shard from a general protobuf node
func AttemptHAMTShardFromNode(ctx context.Context, nd ipld.Node, lsys *ipld.LinkSystem) (UnixFSHAMTShard, error) {
	// shortcut if node is already a hamt
	hnd, ok := nd.(UnixFSHAMTShard)
	if ok {
		return hnd, nil
	}
	pbnd, ok := nd.(dagpb.PBNode)
	if !ok {
		return nil, fmt.Errorf("hamt.AttemptHAMTShardFromNode: %w", ErrNotProtobuf)
	}
	if !pbnd.FieldData().Exists() {
		return nil, fmt.Errorf("hamt.AttemptHAMTShardFromNode: %w", ErrNotUnixFSNode)
	}
	data, err := data.DecodeUnixFSData(pbnd.FieldData().Must().Bytes())
	if err != nil {
		return nil, err
	}
	und, err := NewUnixFSHAMTShard(ctx, pbnd, data, lsys)
	if err != nil {
		return nil, err
	}
	return und.(UnixFSHAMTShard), nil
}

func (n UnixFSHAMTShard) loadChild(pbLink dagpb.PBLink) (UnixFSHAMTShard, error) {
	cached, ok := n.shardCache[pbLink.FieldHash().Link()]
	if ok {
		return cached, nil
	}
	nd, err := n.lsys.Load(ipld.LinkContext{Ctx: n.ctx}, pbLink.FieldHash().Link(), dagpb.Type.PBNode)
	if err != nil {
		return nil, err
	}
	und, err := AttemptHAMTShardFromNode(n.ctx, nd, n.lsys)
	if err != nil {
		return nil, err
	}
	n.shardCache[pbLink.FieldHash().Link()] = und
	return und, nil
}

func (n UnixFSHAMTShard) LookupByNode(key ipld.Node) (ipld.Node, error) {
	ks, err := key.AsString()
	if err != nil {
		return nil, err
	}
	return n.LookupByString(ks)
}

func (n UnixFSHAMTShard) LookupByIndex(idx int64) (ipld.Node, error) {
	return n._substrate.LookupByIndex(idx)
}

func (n UnixFSHAMTShard) LookupBySegment(seg ipld.PathSegment) (ipld.Node, error) {
	return n.LookupByString(seg.String())
}

func (n UnixFSHAMTShard) MapIterator() ipld.MapIterator {
	maxPadLen := maxPadLength(n.data)
	listItr := &_UnixFSShardedDir__ListItr{
		_substrate: n.FieldLinks().Iterator(),
		maxPadLen:  maxPadLen,
		nd:         n,
	}
	st := stringTransformer{maxPadLen: maxPadLen}
	return iter.NewUnixFSDirMapIterator(listItr, st.transformNameNode)
}

type _UnixFSShardedDir__ListItr struct {
	_substrate *dagpb.PBLinks__Itr
	childIter  *_UnixFSShardedDir__ListItr
	nd         UnixFSHAMTShard
	maxPadLen  int
	total      int64
}

func (itr *_UnixFSShardedDir__ListItr) Next() (int64, dagpb.PBLink) {
	next := itr.next()
	if next == nil {
		return -1, next
	}
	total := itr.total
	itr.total++
	return total, next
}

func (itr *_UnixFSShardedDir__ListItr) next() dagpb.PBLink {

	if itr.childIter == nil {
		if itr._substrate.Done() {
			return nil
		}
		_, next := itr._substrate.Next()
		isValue, err := isValueLink(next, itr.maxPadLen)
		if err != nil {
			return nil
		}
		if isValue {
			return next
		}
		child, err := itr.nd.loadChild(next)
		if err != nil {
			return nil
		}
		itr.childIter = &_UnixFSShardedDir__ListItr{
			_substrate: child._substrate.FieldLinks().Iterator(),
			nd:         child,
			maxPadLen:  maxPadLength(child.data),
		}

	}
	_, next := itr.childIter.Next()
	if itr.childIter.Done() {
		itr.childIter = nil
	}
	return next
}

func (itr *_UnixFSShardedDir__ListItr) Done() bool {
	return itr.childIter == nil && itr._substrate.Done()
}

// ListIterator returns an iterator which yields key-value pairs
// traversing the node.
// If the node kind is anything other than a list, nil will be returned.
//
// The iterator will yield every entry in the list; that is, it
// can be expected that itr.Next will be called node.Length times
// before itr.Done becomes true.
func (n UnixFSHAMTShard) ListIterator() ipld.ListIterator {
	return nil
}

// Length returns the length of a list, or the number of entries in a map,
// or -1 if the node is not of list nor map kind.
func (n UnixFSHAMTShard) Length() int64 {
	if n.cachedLength != -1 {
		return n.cachedLength
	}
	maxPadLen := maxPadLength(n.data)
	total := int64(0)
	itr := n.FieldLinks().Iterator()
	for !itr.Done() {
		_, pbLink := itr.Next()
		isValue, err := isValueLink(pbLink, maxPadLen)
		if err != nil {
			continue
		}
		if isValue {
			total++
		} else {
			child, err := n.loadChild(pbLink)
			if err != nil {
				continue
			}
			total += child.Length()
		}
	}
	n.cachedLength = total
	return total
}

func (n UnixFSHAMTShard) IsAbsent() bool {
	return false
}

func (n UnixFSHAMTShard) IsNull() bool {
	return false
}

func (n UnixFSHAMTShard) AsBool() (bool, error) {
	return n._substrate.AsBool()
}

func (n UnixFSHAMTShard) AsInt() (int64, error) {
	return n._substrate.AsInt()
}

func (n UnixFSHAMTShard) AsFloat() (float64, error) {
	return n._substrate.AsFloat()
}

func (n UnixFSHAMTShard) AsString() (string, error) {
	return n._substrate.AsString()
}

func (n UnixFSHAMTShard) AsBytes() ([]byte, error) {
	return n._substrate.AsBytes()
}

func (n UnixFSHAMTShard) AsLink() (ipld.Link, error) {
	return n._substrate.AsLink()
}

func (n UnixFSHAMTShard) Prototype() ipld.NodePrototype {
	// TODO: should this return something?
	// probobly not until we write the write interfaces
	return nil
}

// satisfy schema.TypedNode
func (UnixFSHAMTShard) Type() schema.Type {
	return nil /*TODO:typelit*/
}

func (n UnixFSHAMTShard) Representation() ipld.Node {
	return n._substrate.Representation()
}

// Native map accessors

func (n UnixFSHAMTShard) Iterator() *iter.UnixFSDir__Itr {
	maxPadLen := maxPadLength(n.data)
	listItr := &_UnixFSShardedDir__ListItr{
		_substrate: n.FieldLinks().Iterator(),
		maxPadLen:  maxPadLen,
		nd:         n,
	}
	st := stringTransformer{maxPadLen: maxPadLen}
	return iter.NewUnixFSDirIterator(listItr, st.transformNameNode)
}

func (n UnixFSHAMTShard) Lookup(key dagpb.String) dagpb.Link {
	hv := &hashBits{b: hash([]byte(key.String()))}
	link, err := n.lookup(key.String(), hv)
	if err != nil {
		return nil
	}
	return link
}

// direct access to the links and data

func (n UnixFSHAMTShard) FieldLinks() dagpb.PBLinks {
	return n._substrate.FieldLinks()
}

func (n UnixFSHAMTShard) FieldData() dagpb.MaybeBytes {
	return n._substrate.FieldData()
}

func (n UnixFSHAMTShard) getChildLink(childIndex int) (dagpb.PBLink, error) {
	linkIndex := n.bitfield.OnesBefore(childIndex)
	if linkIndex >= int(n.FieldLinks().Length()) || linkIndex < 0 {
		return nil, ErrInvalidChildIndex
	}
	return n.FieldLinks().Lookup(int64(linkIndex)), nil
}

func (n UnixFSHAMTShard) hasChild(childIndex int) bool {
	return n.bitfield.Bit(childIndex)
}

type stringTransformer struct {
	maxPadLen int
}

func (s stringTransformer) transformNameNode(nd dagpb.String) dagpb.String {
	nb := dagpb.Type.String.NewBuilder()
	err := nb.AssignString(nd.String()[s.maxPadLen:])
	if err != nil {
		return nil
	}
	return nb.Build().(dagpb.String)
}
