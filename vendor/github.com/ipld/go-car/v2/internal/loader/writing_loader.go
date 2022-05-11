package loader

import (
	"bytes"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car/v2/index"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/linking"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-varint"
)

type writerOutput struct {
	w     io.Writer
	size  uint64
	code  multicodec.Code
	rcrds []index.Record
}

func (w *writerOutput) Size() uint64 {
	return w.size
}

func (w *writerOutput) Index() (index.Index, error) {
	idx, err := index.New(w.code)
	if err != nil {
		return nil, err
	}
	if err := idx.Load(w.rcrds); err != nil {
		return nil, err
	}

	return idx, nil
}

// An IndexTracker tracks the records loaded/written, calculate an
// index based on them.
type IndexTracker interface {
	ReadCounter
	Index() (index.Index, error)
}

type writingReader struct {
	r   io.Reader
	len int64
	cid string
	wo  *writerOutput
}

func (w *writingReader) Read(p []byte) (int, error) {
	if w.wo != nil {
		// write the cid
		size := varint.ToUvarint(uint64(w.len) + uint64(len(w.cid)))
		if _, err := w.wo.w.Write(size); err != nil {
			return 0, err
		}
		if _, err := w.wo.w.Write([]byte(w.cid)); err != nil {
			return 0, err
		}
		cpy := bytes.NewBuffer(w.r.(*bytes.Buffer).Bytes())
		if _, err := cpy.WriteTo(w.wo.w); err != nil {
			return 0, err
		}

		// maybe write the index.
		if w.wo.code != index.CarIndexNone {
			_, c, err := cid.CidFromBytes([]byte(w.cid))
			if err != nil {
				return 0, err
			}
			w.wo.rcrds = append(w.wo.rcrds, index.Record{
				Cid:    c,
				Offset: w.wo.size,
			})
		}
		w.wo.size += uint64(w.len) + uint64(len(size)+len(w.cid))

		w.wo = nil
	}

	return w.r.Read(p)
}

// TeeingLinkSystem wraps an IPLD.LinkSystem so that each time a block is loaded from it,
// that block is also written as a CAR block to the provided io.Writer. Metadata
// (the size of data written) is provided in the second return value.
// The `initialOffset` is used to calculate the offsets recorded for the index, and will be
//   included in the `.Size()` of the IndexTracker.
// An indexCodec of `index.CarIndexNoIndex` can be used to not track these offsets.
func TeeingLinkSystem(ls ipld.LinkSystem, w io.Writer, initialOffset uint64, indexCodec multicodec.Code) (ipld.LinkSystem, IndexTracker) {
	wo := writerOutput{
		w:     w,
		size:  initialOffset,
		code:  indexCodec,
		rcrds: make([]index.Record, 0),
	}

	tls := ls
	tls.StorageReadOpener = func(lc linking.LinkContext, l ipld.Link) (io.Reader, error) {
		r, err := ls.StorageReadOpener(lc, l)
		if err != nil {
			return nil, err
		}
		buf := bytes.NewBuffer(nil)
		n, err := buf.ReadFrom(r)
		if err != nil {
			return nil, err
		}
		return &writingReader{buf, n, l.Binary(), &wo}, nil
	}
	return tls, &wo
}
