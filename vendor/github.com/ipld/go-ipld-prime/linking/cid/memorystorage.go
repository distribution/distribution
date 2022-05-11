package cidlink

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/linking"
)

// Memory is a simple in-memory storage for cidlinks. It's the same as `storage.Memory`
// but uses typical multihash semantics used when reading/writing cidlinks.
//
// Using multihash as the storage key rather than the whole CID will remove the
// distinction between CIDv0 and their CIDv1 counterpart. It also removes the
// distinction between CIDs where the multihash is the same but the codec is
// different, e.g. `dag-cbor` and a `raw` version of the same data.
type Memory struct {
	Bag map[string][]byte
}

func (store *Memory) beInitialized() {
	if store.Bag != nil {
		return
	}
	store.Bag = make(map[string][]byte)
}

func (store *Memory) OpenRead(lnkCtx linking.LinkContext, lnk datamodel.Link) (io.Reader, error) {
	store.beInitialized()
	cl, ok := lnk.(Link)
	if !ok {
		return nil, fmt.Errorf("incompatible link type: %T", lnk)
	}
	data, exists := store.Bag[string(cl.Hash())]
	if !exists {
		return nil, os.ErrNotExist
	}
	return bytes.NewReader(data), nil
}

func (store *Memory) OpenWrite(lnkCtx linking.LinkContext) (io.Writer, linking.BlockWriteCommitter, error) {
	store.beInitialized()
	buf := bytes.Buffer{}
	return &buf, func(lnk datamodel.Link) error {
		cl, ok := lnk.(Link)
		if !ok {
			return fmt.Errorf("incompatible link type: %T", lnk)
		}

		store.Bag[string(cl.Hash())] = buf.Bytes()
		return nil
	}, nil
}
