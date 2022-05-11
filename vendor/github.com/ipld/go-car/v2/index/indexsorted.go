package index

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/multiformats/go-multicodec"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

var _ Index = (*multiWidthIndex)(nil)

type (
	digestRecord struct {
		digest []byte
		index  uint64
	}
	recordSet        []digestRecord
	singleWidthIndex struct {
		width uint32
		len   uint64 // in struct, len is #items. when marshaled, it's saved as #bytes.
		index []byte
	}
	multiWidthIndex map[uint32]singleWidthIndex
)

func (d digestRecord) write(buf []byte) {
	n := copy(buf[:], d.digest)
	binary.LittleEndian.PutUint64(buf[n:], d.index)
}

func (r recordSet) Len() int {
	return len(r)
}

func (r recordSet) Less(i, j int) bool {
	return bytes.Compare(r[i].digest, r[j].digest) < 0
}

func (r recordSet) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (s *singleWidthIndex) Marshal(w io.Writer) (uint64, error) {
	l := uint64(0)
	if err := binary.Write(w, binary.LittleEndian, s.width); err != nil {
		return 0, err
	}
	l += 4
	if err := binary.Write(w, binary.LittleEndian, int64(len(s.index))); err != nil {
		return l, err
	}
	l += 8
	n, err := w.Write(s.index)
	return l + uint64(n), err
}

func (s *singleWidthIndex) Unmarshal(r io.Reader) error {
	if err := binary.Read(r, binary.LittleEndian, &s.width); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &s.len); err != nil {
		return err
	}
	s.index = make([]byte, s.len)
	s.len /= uint64(s.width)
	_, err := io.ReadFull(r, s.index)
	return err
}

func (s *singleWidthIndex) Less(i int, digest []byte) bool {
	return bytes.Compare(digest[:], s.index[i*int(s.width):((i+1)*int(s.width)-8)]) <= 0
}

func (s *singleWidthIndex) GetAll(c cid.Cid, fn func(uint64) bool) error {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return err
	}
	return s.getAll(d.Digest, fn)
}

func (s *singleWidthIndex) getAll(d []byte, fn func(uint64) bool) error {
	idx := sort.Search(int(s.len), func(i int) bool {
		return s.Less(i, d)
	})

	var any bool
	for ; uint64(idx) < s.len; idx++ {
		digestStart := idx * int(s.width)
		offsetEnd := (idx + 1) * int(s.width)
		digestEnd := offsetEnd - 8
		if bytes.Equal(d[:], s.index[digestStart:digestEnd]) {
			any = true
			offset := binary.LittleEndian.Uint64(s.index[digestEnd:offsetEnd])
			if !fn(offset) {
				// User signalled to stop searching; therefore, break.
				break
			}
		} else {
			// No more matches; therefore, break.
			break
		}
	}
	if !any {
		return ErrNotFound
	}
	return nil
}

func (s *singleWidthIndex) Load(items []Record) error {
	m := make(multiWidthIndex)
	if err := m.Load(items); err != nil {
		return err
	}
	if len(m) != 1 {
		return fmt.Errorf("unexpected number of cid widths: %d", len(m))
	}
	for _, i := range m {
		s.index = i.index
		s.len = i.len
		s.width = i.width
		return nil
	}
	return nil
}

func (s *singleWidthIndex) forEachDigest(f func(digest []byte, offset uint64) error) error {
	segmentCount := len(s.index) / int(s.width)
	for i := 0; i < segmentCount; i++ {
		digestStart := i * int(s.width)
		offsetEnd := (i + 1) * int(s.width)
		digestEnd := offsetEnd - 8
		digest := s.index[digestStart:digestEnd]
		offset := binary.LittleEndian.Uint64(s.index[digestEnd:offsetEnd])
		if err := f(digest, offset); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiWidthIndex) GetAll(c cid.Cid, fn func(uint64) bool) error {
	d, err := multihash.Decode(c.Hash())
	if err != nil {
		return err
	}
	if s, ok := (*m)[uint32(len(d.Digest)+8)]; ok {
		return s.getAll(d.Digest, fn)
	}
	return ErrNotFound
}

func (m *multiWidthIndex) Codec() multicodec.Code {
	return multicodec.CarIndexSorted
}

func (m *multiWidthIndex) Marshal(w io.Writer) (uint64, error) {
	l := uint64(0)
	if err := binary.Write(w, binary.LittleEndian, int32(len(*m))); err != nil {
		return l, err
	}
	l += 4

	// The widths are unique, but ranging over a map isn't deterministic.
	// As per the CARv2 spec, we must order buckets by digest length.

	widths := make([]uint32, 0, len(*m))
	for width := range *m {
		widths = append(widths, width)
	}
	sort.Slice(widths, func(i, j int) bool {
		return widths[i] < widths[j]
	})

	for _, width := range widths {
		bucket := (*m)[width]
		n, err := bucket.Marshal(w)
		l += n
		if err != nil {
			return l, err
		}
	}
	return l, nil
}

func (m *multiWidthIndex) Unmarshal(r io.Reader) error {
	var l int32
	if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
		return err
	}
	for i := 0; i < int(l); i++ {
		s := singleWidthIndex{}
		if err := s.Unmarshal(r); err != nil {
			return err
		}
		(*m)[s.width] = s
	}
	return nil
}

func (m *multiWidthIndex) Load(items []Record) error {
	// Split cids on their digest length
	idxs := make(map[int][]digestRecord)
	for _, item := range items {
		decHash, err := multihash.Decode(item.Hash())
		if err != nil {
			return err
		}

		digest := decHash.Digest
		idx, ok := idxs[len(digest)]
		if !ok {
			idxs[len(digest)] = make([]digestRecord, 0)
			idx = idxs[len(digest)]
		}
		idxs[len(digest)] = append(idx, digestRecord{digest, item.Offset})
	}

	// Sort each list. then write to compact form.
	for width, lst := range idxs {
		sort.Sort(recordSet(lst))
		rcrdWdth := width + 8
		compact := make([]byte, rcrdWdth*len(lst))
		for off, itm := range lst {
			itm.write(compact[off*rcrdWdth : (off+1)*rcrdWdth])
		}
		s := singleWidthIndex{
			width: uint32(rcrdWdth),
			len:   uint64(len(lst)),
			index: compact,
		}
		(*m)[uint32(width)+8] = s
	}
	return nil
}

func (m *multiWidthIndex) forEachDigest(f func(digest []byte, offset uint64) error) error {
	sizes := make([]uint32, 0, len(*m))
	for k := range *m {
		sizes = append(sizes, k)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	for _, s := range sizes {
		swi := (*m)[s]
		if err := swi.forEachDigest(f); err != nil {
			return err
		}
	}
	return nil
}

func newSorted() Index {
	m := make(multiWidthIndex)
	return &m
}
