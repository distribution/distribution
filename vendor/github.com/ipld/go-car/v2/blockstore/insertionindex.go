package blockstore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/index"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/petar/GoLLRB/llrb"
	cbor "github.com/whyrusleeping/cbor/go"
)

var (
	errUnsupported      = errors.New("not supported")
	insertionIndexCodec = multicodec.Code(0x300003)
)

type (
	insertionIndex struct {
		items llrb.LLRB
	}

	recordDigest struct {
		digest []byte
		index.Record
	}
)

func (r recordDigest) Less(than llrb.Item) bool {
	other, ok := than.(recordDigest)
	if !ok {
		return false
	}
	return bytes.Compare(r.digest, other.digest) < 0
}

func newRecordDigest(r index.Record) recordDigest {
	d, err := multihash.Decode(r.Hash())
	if err != nil {
		panic(err)
	}

	return recordDigest{d.Digest, r}
}

func newRecordFromCid(c cid.Cid, at uint64) recordDigest {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		panic(err)
	}

	return recordDigest{d.Digest, index.Record{Cid: c, Offset: at}}
}

func (ii *insertionIndex) insertNoReplace(key cid.Cid, n uint64) {
	ii.items.InsertNoReplace(newRecordFromCid(key, n))
}

func (ii *insertionIndex) Get(c cid.Cid) (uint64, error) {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return 0, err
	}
	entry := recordDigest{digest: d.Digest}
	e := ii.items.Get(entry)
	if e == nil {
		return 0, index.ErrNotFound
	}
	r, ok := e.(recordDigest)
	if !ok {
		return 0, errUnsupported
	}

	return r.Record.Offset, nil
}

func (ii *insertionIndex) GetAll(c cid.Cid, fn func(uint64) bool) error {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return err
	}
	entry := recordDigest{digest: d.Digest}

	any := false
	iter := func(i llrb.Item) bool {
		existing := i.(recordDigest)
		if !bytes.Equal(existing.digest, entry.digest) {
			// We've already looked at all entries with matching digests.
			return false
		}
		any = true
		return fn(existing.Record.Offset)
	}
	ii.items.AscendGreaterOrEqual(entry, iter)
	if !any {
		return index.ErrNotFound
	}
	return nil
}

func (ii *insertionIndex) Marshal(w io.Writer) (uint64, error) {
	l := uint64(0)
	if err := binary.Write(w, binary.LittleEndian, int64(ii.items.Len())); err != nil {
		return l, err
	}
	l += 8

	var err error
	iter := func(i llrb.Item) bool {
		if err = cbor.Encode(w, i.(recordDigest).Record); err != nil {
			return false
		}
		return true
	}
	ii.items.AscendGreaterOrEqual(ii.items.Min(), iter)
	return l, err
}

func (ii *insertionIndex) Unmarshal(r io.Reader) error {
	var length int64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return err
	}
	d := cbor.NewDecoder(r)
	for i := int64(0); i < length; i++ {
		var rec index.Record
		if err := d.Decode(&rec); err != nil {
			return err
		}
		ii.items.InsertNoReplace(newRecordDigest(rec))
	}
	return nil
}

func (ii *insertionIndex) Codec() multicodec.Code {
	return insertionIndexCodec
}

func (ii *insertionIndex) Load(rs []index.Record) error {
	for _, r := range rs {
		rec := newRecordDigest(r)
		if rec.digest == nil {
			return fmt.Errorf("invalid entry: %v", r)
		}
		ii.items.InsertNoReplace(rec)
	}
	return nil
}

func newInsertionIndex() *insertionIndex {
	return &insertionIndex{}
}

// flatten returns a formatted index in the given codec for more efficient subsequent loading.
func (ii *insertionIndex) flatten(codec multicodec.Code) (index.Index, error) {
	si, err := index.New(codec)
	if err != nil {
		return nil, err
	}
	rcrds := make([]index.Record, ii.items.Len())

	idx := 0
	iter := func(i llrb.Item) bool {
		rcrds[idx] = i.(recordDigest).Record
		idx++
		return true
	}
	ii.items.AscendGreaterOrEqual(ii.items.Min(), iter)

	if err := si.Load(rcrds); err != nil {
		return nil, err
	}
	return si, nil
}

// note that hasExactCID is very similar to GetAll,
// but it's separate as it allows us to compare Record.Cid directly,
// whereas GetAll just provides Record.Offset.

func (ii *insertionIndex) hasExactCID(c cid.Cid) bool {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		panic(err)
	}
	entry := recordDigest{digest: d.Digest}

	found := false
	iter := func(i llrb.Item) bool {
		existing := i.(recordDigest)
		if !bytes.Equal(existing.digest, entry.digest) {
			// We've already looked at all entries with matching digests.
			return false
		}
		if existing.Record.Cid == c {
			// We found an exact match.
			found = true
			return false
		}
		// Continue looking in ascending order.
		return true
	}
	ii.items.AscendGreaterOrEqual(entry, iter)
	return found
}
