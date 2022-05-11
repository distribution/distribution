package builder

import (
	"fmt"
	"hash"

	bitfield "github.com/ipfs/go-bitfield"
	"github.com/ipfs/go-unixfsnode/data"
	"github.com/ipfs/go-unixfsnode/hamt"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/multiformats/go-multihash"
	"github.com/spaolacci/murmur3"
)

type shard struct {
	// metadata about the shard
	hasher  uint64
	size    int
	sizeLg2 int
	width   int
	depth   int

	children map[int]entry
}

// a shard entry is either another shard, or a direct link.
type entry struct {
	*shard
	*hamtLink
}

// a hamtLink is a member of the hamt - the file/directory pointed to, but
// stored with it's hashed key used for addressing.
type hamtLink struct {
	hash hashBits
	dagpb.PBLink
}

// BuildUnixFSShardedDirectory will build a hamt of unixfs hamt shards encoing a directory with more entries
// than is typically allowed to fit in a standard IPFS single-block unixFS directory.
func BuildUnixFSShardedDirectory(size int, hasher uint64, entries []dagpb.PBLink, ls *ipld.LinkSystem) (ipld.Link, uint64, error) {
	// hash the entries
	var h hash.Hash
	var err error
	// TODO: use the multihash registry once murmur3 behavior is encoded there.
	// https://github.com/multiformats/go-multihash/pull/150
	if hasher == hamt.HashMurmur3 {
		h = murmur3.New64()
	} else {
		h, err = multihash.GetHasher(hasher)
		if err != nil {
			return nil, 0, err
		}
	}
	hamtEntries := make([]hamtLink, 0, len(entries))
	for _, e := range entries {
		name := e.Name.Must().String()
		h.Reset()
		h.Write([]byte(name))
		sum := h.Sum(nil)
		hamtEntries = append(hamtEntries, hamtLink{
			sum,
			e,
		})
	}

	sizeLg2, err := logtwo(size)
	if err != nil {
		return nil, 0, err
	}

	sharder := shard{
		hasher:  hasher,
		size:    size,
		sizeLg2: sizeLg2,
		width:   len(fmt.Sprintf("%X", size-1)),
		depth:   0,

		children: make(map[int]entry),
	}

	for _, entry := range hamtEntries {
		err := sharder.add(entry)
		if err != nil {
			return nil, 0, err
		}
	}

	return sharder.serialize(ls)
}

func (s *shard) add(lnk hamtLink) error {
	// get the bucket for lnk
	bucket, err := lnk.hash.Slice(s.depth*s.sizeLg2, s.sizeLg2)
	if err != nil {
		return err
	}

	current, ok := s.children[bucket]
	if !ok {
		// no bucket, make one with this entry
		s.children[bucket] = entry{nil, &lnk}
		return nil
	} else if current.shard != nil {
		// existing shard, add this link to the shard
		return current.shard.add(lnk)
	}
	// make a shard for current and lnk
	newShard := entry{
		&shard{
			hasher:   s.hasher,
			size:     s.size,
			sizeLg2:  s.sizeLg2,
			width:    s.width,
			depth:    s.depth + 1,
			children: make(map[int]entry),
		},
		nil,
	}
	// add existing link from this bucket to the new shard
	if err := newShard.add(*current.hamtLink); err != nil {
		return err
	}
	// replace bucket with shard
	s.children[bucket] = newShard
	// add new link to the new shard
	return newShard.add(lnk)
}

func (s *shard) formatLinkName(name string, idx int) string {
	return fmt.Sprintf("%0*X%s", s.width, idx, name)
}

// bitmap calculates the bitmap of which links in the shard are set.
func (s *shard) bitmap() []byte {
	bm := bitfield.NewBitfield(s.size)
	for i := 0; i < s.size; i++ {
		if _, ok := s.children[i]; ok {
			bm.SetBit(i)
		}
	}
	return bm.Bytes()
}

// serialize stores the concrete representation of this shard in the link system and
// returns a link to it.
func (s *shard) serialize(ls *ipld.LinkSystem) (ipld.Link, uint64, error) {
	ufd, err := BuildUnixFS(func(b *Builder) {
		DataType(b, data.Data_HAMTShard)
		HashType(b, s.hasher)
		Data(b, s.bitmap())
		Fanout(b, uint64(s.size))
	})
	if err != nil {
		return nil, 0, err
	}
	pbb := dagpb.Type.PBNode.NewBuilder()
	pbm, err := pbb.BeginMap(2)
	if err != nil {
		return nil, 0, err
	}
	if err = pbm.AssembleKey().AssignString("Data"); err != nil {
		return nil, 0, err
	}
	if err = pbm.AssembleValue().AssignBytes(data.EncodeUnixFSData(ufd)); err != nil {
		return nil, 0, err
	}
	if err = pbm.AssembleKey().AssignString("Links"); err != nil {
		return nil, 0, err
	}

	lnkBuilder := dagpb.Type.PBLinks.NewBuilder()
	lnks, err := lnkBuilder.BeginList(int64(len(s.children)))
	if err != nil {
		return nil, 0, err
	}
	// sorting happens in codec-dagpb
	var totalSize uint64
	for idx, e := range s.children {
		var lnk dagpb.PBLink
		if e.shard != nil {
			ipldLnk, sz, err := e.shard.serialize(ls)
			if err != nil {
				return nil, 0, err
			}
			totalSize += sz
			fullName := s.formatLinkName("", idx)
			lnk, err = BuildUnixFSDirectoryEntry(fullName, int64(sz), ipldLnk)
			if err != nil {
				return nil, 0, err
			}
		} else {
			fullName := s.formatLinkName(e.Name.Must().String(), idx)
			sz := e.Tsize.Must().Int()
			totalSize += uint64(sz)
			lnk, err = BuildUnixFSDirectoryEntry(fullName, sz, e.Hash.Link())
		}
		if err != nil {
			return nil, 0, err
		}
		if err := lnks.AssembleValue().AssignNode(lnk); err != nil {
			return nil, 0, err
		}
	}
	if err := lnks.Finish(); err != nil {
		return nil, 0, err
	}
	pbm.AssembleValue().AssignNode(lnkBuilder.Build())
	if err := pbm.Finish(); err != nil {
		return nil, 0, err
	}
	node := pbb.Build()
	lnk, sz, err := sizedStore(ls, fileLinkProto, node)
	if err != nil {
		return nil, 0, err
	}
	return lnk, totalSize + sz, nil
}
