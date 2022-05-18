package estuary

import (
	"bytes"
	"io"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	"github.com/ipfs/go-merkledag"

	chunker "github.com/ipfs/go-ipfs-chunker"
	ipld "github.com/ipfs/go-ipld-format"
	importer "github.com/ipfs/go-unixfs/importer"

	"github.com/ipfs/go-unixfs/importer/balanced"
	helper "github.com/ipfs/go-unixfs/importer/helpers"
	mh "github.com/multiformats/go-multihash"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	bs "github.com/ipfs/go-ipfs-blockstore"
)

const DEFAULT_CHUNK_SIZE = 1024 * 1024

func rootCid0(reader io.Reader) (ipld.Node, error) {
	bs := bs.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	bserv := blockservice.New(bs, nil)
	dserv := merkledag.NewDAGService(bserv)
	return importer.BuildDagFromReader(dserv, chunker.DefaultSplitter(reader))
}

func rootCid1(contentReader io.Reader) (ipld.Node, error) {
	bs := bs.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	bserv := blockservice.New(bs, nil)
	dserv := merkledag.NewDAGService(bserv)
	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return nil, err
	}
	prefix.MhType = uint64(mh.SHA2_256)
	spl := chunker.NewSizeSplitter(contentReader, DEFAULT_CHUNK_SIZE)
	dbp := helper.DagBuilderParams{
		Maxlinks:  1024,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   32,
		},
		Dagserv: dserv,
	}
	db, err := dbp.New(spl)
	if err != nil {
		return nil, err
	}
	return balanced.Layout(db)
}

func SumCid(contents []byte) (cid.Cid, error) {
	nd, err := rootCid1(bytes.NewReader(contents))
	if err != nil {
		return cid.Undef, err
	}
	return nd.Cid(), nil
}

func Sum(contents []byte) ([]byte, error) {
	c, err := SumCid(contents)
	if err != nil {
		return nil, err
	}
	return c.Bytes(), nil
}

func EncodeToString(contents []byte) string {
	_, c, err := cid.CidFromBytes(contents)
	if err != nil {
		panic(err)
	}
	return c.String()
}
