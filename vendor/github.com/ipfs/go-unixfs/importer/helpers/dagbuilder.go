package helpers

import (
	"context"
	"errors"
	"io"
	"os"

	dag "github.com/ipfs/go-merkledag"

	ft "github.com/ipfs/go-unixfs"
	pb "github.com/ipfs/go-unixfs/pb"

	cid "github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	files "github.com/ipfs/go-ipfs-files"
	pi "github.com/ipfs/go-ipfs-posinfo"
	ipld "github.com/ipfs/go-ipld-format"
)

var ErrMissingFsRef = errors.New("missing file path or URL, can't create filestore reference")

// DagBuilderHelper wraps together a bunch of objects needed to
// efficiently create unixfs dag trees
type DagBuilderHelper struct {
	dserv      ipld.DAGService
	spl        chunker.Splitter
	recvdErr   error
	rawLeaves  bool
	nextData   []byte // the next item to return.
	maxlinks   int
	cidBuilder cid.Builder

	// Filestore support variables.
	// ----------------------------
	// TODO: Encapsulate in `FilestoreNode` (which is basically what they are).
	//
	// Besides having the path this variable (if set) is used as a flag
	// to indicate that Filestore should be used.
	fullPath string
	stat     os.FileInfo
	// Keeps track of the current file size added to the DAG (used in
	// the balanced builder). It is assumed that the `DagBuilderHelper`
	// is not reused to construct another DAG, but a new one (with a
	// zero `offset`) is created.
	offset uint64
}

// DagBuilderParams wraps configuration options to create a DagBuilderHelper
// from a chunker.Splitter.
type DagBuilderParams struct {
	// Maximum number of links per intermediate node
	Maxlinks int

	// RawLeaves signifies that the importer should use raw ipld nodes as leaves
	// instead of using the unixfs TRaw type
	RawLeaves bool

	// CID Builder to use if set
	CidBuilder cid.Builder

	// DAGService to write blocks to (required)
	Dagserv ipld.DAGService

	// NoCopy signals to the chunker that it should track fileinfo for
	// filestore adds
	NoCopy bool
}

// New generates a new DagBuilderHelper from the given params and a given
// chunker.Splitter as data source.
func (dbp *DagBuilderParams) New(spl chunker.Splitter) (*DagBuilderHelper, error) {
	db := &DagBuilderHelper{
		dserv:      dbp.Dagserv,
		spl:        spl,
		rawLeaves:  dbp.RawLeaves,
		cidBuilder: dbp.CidBuilder,
		maxlinks:   dbp.Maxlinks,
	}
	if fi, ok := spl.Reader().(files.FileInfo); dbp.NoCopy && ok {
		db.fullPath = fi.AbsPath()
		db.stat = fi.Stat()
	}

	if dbp.NoCopy && db.fullPath == "" { // Enforce NoCopy
		return nil, ErrMissingFsRef
	}

	return db, nil
}

// prepareNext consumes the next item from the splitter and puts it
// in the nextData field. it is idempotent-- if nextData is full
// it will do nothing.
func (db *DagBuilderHelper) prepareNext() {
	// if we already have data waiting to be consumed, we're ready
	if db.nextData != nil || db.recvdErr != nil {
		return
	}

	db.nextData, db.recvdErr = db.spl.NextBytes()
	if db.recvdErr == io.EOF {
		db.recvdErr = nil
	}
}

// Done returns whether or not we're done consuming the incoming data.
func (db *DagBuilderHelper) Done() bool {
	// ensure we have an accurate perspective on data
	// as `done` this may be called before `next`.
	db.prepareNext() // idempotent
	if db.recvdErr != nil {
		return false
	}
	return db.nextData == nil
}

// Next returns the next chunk of data to be inserted into the dag
// if it returns nil, that signifies that the stream is at an end, and
// that the current building operation should finish.
func (db *DagBuilderHelper) Next() ([]byte, error) {
	db.prepareNext() // idempotent
	d := db.nextData
	db.nextData = nil // signal we've consumed it
	if db.recvdErr != nil {
		return nil, db.recvdErr
	}
	return d, nil
}

// GetDagServ returns the dagservice object this Helper is using
func (db *DagBuilderHelper) GetDagServ() ipld.DAGService {
	return db.dserv
}

// GetCidBuilder returns the internal `cid.CidBuilder` set in the builder.
func (db *DagBuilderHelper) GetCidBuilder() cid.Builder {
	return db.cidBuilder
}

// NewLeafNode creates a leaf node filled with data.  If rawLeaves is
// defined then a raw leaf will be returned.  Otherwise, it will create
// and return `FSNodeOverDag` with `fsNodeType`.
func (db *DagBuilderHelper) NewLeafNode(data []byte, fsNodeType pb.Data_DataType) (ipld.Node, error) {
	if len(data) > BlockSizeLimit {
		return nil, ErrSizeLimitExceeded
	}

	if db.rawLeaves {
		// Encapsulate the data in a raw node.
		if db.cidBuilder == nil {
			return dag.NewRawNode(data), nil
		}
		rawnode, err := dag.NewRawNodeWPrefix(data, db.cidBuilder)
		if err != nil {
			return nil, err
		}
		return rawnode, nil
	}

	// Encapsulate the data in UnixFS node (instead of a raw node).
	fsNodeOverDag := db.NewFSNodeOverDag(fsNodeType)
	fsNodeOverDag.SetFileData(data)
	node, err := fsNodeOverDag.Commit()
	if err != nil {
		return nil, err
	}
	// TODO: Encapsulate this sequence of calls into a function that
	// just returns the final `ipld.Node` avoiding going through
	// `FSNodeOverDag`.

	return node, nil
}

// FillNodeLayer will add datanodes as children to the give node until
// it is full in this layer or no more data.
// NOTE: This function creates raw data nodes so it only works
// for the `trickle.Layout`.
func (db *DagBuilderHelper) FillNodeLayer(node *FSNodeOverDag) error {

	// while we have room AND we're not done
	for node.NumChildren() < db.maxlinks && !db.Done() {
		child, childFileSize, err := db.NewLeafDataNode(ft.TRaw)
		if err != nil {
			return err
		}

		if err := node.AddChild(child, childFileSize, db); err != nil {
			return err
		}
	}
	node.Commit()
	// TODO: Do we need to commit here? The caller who created the
	// `FSNodeOverDag` should be in charge of that.

	return nil
}

// NewLeafDataNode builds the `node` with the data obtained from the
// Splitter with the given constraints (BlockSizeLimit, RawLeaves)
// specified when creating the DagBuilderHelper. It returns
// `ipld.Node` with the `dataSize` (that will be used to keep track of
// the DAG file size). The size of the data is computed here because
// after that it will be hidden by `NewLeafNode` inside a generic
// `ipld.Node` representation.
func (db *DagBuilderHelper) NewLeafDataNode(fsNodeType pb.Data_DataType) (node ipld.Node, dataSize uint64, err error) {
	fileData, err := db.Next()
	if err != nil {
		return nil, 0, err
	}
	dataSize = uint64(len(fileData))

	// Create a new leaf node containing the file chunk data.
	node, err = db.NewLeafNode(fileData, fsNodeType)
	if err != nil {
		return nil, 0, err
	}

	// Convert this leaf to a `FilestoreNode` if needed.
	node = db.ProcessFileStore(node, dataSize)

	return node, dataSize, nil
}

// ProcessFileStore generates, if Filestore is being used, the
// `FilestoreNode` representation of the `ipld.Node` that
// contains the file data. If Filestore is not being used just
// return the same node to continue with its addition to the DAG.
//
// The `db.offset` is updated at this point (instead of when
// `NewLeafDataNode` is called, both work in tandem but the
// offset is more related to this function).
func (db *DagBuilderHelper) ProcessFileStore(node ipld.Node, dataSize uint64) ipld.Node {
	// Check if Filestore is being used.
	if db.fullPath != "" {
		// Check if the node is actually a raw node (needed for
		// Filestore support).
		if _, ok := node.(*dag.RawNode); ok {
			fn := &pi.FilestoreNode{
				Node: node,
				PosInfo: &pi.PosInfo{
					Offset:   db.offset,
					FullPath: db.fullPath,
					Stat:     db.stat,
				},
			}

			// Update `offset` with the size of the data generated by `db.Next`.
			db.offset += dataSize

			return fn
		}
	}

	// Filestore is not used, return the same `node` argument.
	return node
}

// Add inserts the given node in the DAGService.
func (db *DagBuilderHelper) Add(node ipld.Node) error {
	return db.dserv.Add(context.TODO(), node)
}

// Maxlinks returns the configured maximum number for links
// for nodes built with this helper.
func (db *DagBuilderHelper) Maxlinks() int {
	return db.maxlinks
}

// FSNodeOverDag encapsulates an `unixfs.FSNode` that will be stored in a
// `dag.ProtoNode`. Instead of just having a single `ipld.Node` that
// would need to be constantly (un)packed to access and modify its
// internal `FSNode` in the process of creating a UnixFS DAG, this
// structure stores an `FSNode` cache to manipulate it (add child nodes)
// directly , and only when the node has reached its final (immutable) state
// (signaled by calling `Commit()`) is it committed to a single (indivisible)
// `ipld.Node`.
//
// It is used mainly for internal (non-leaf) nodes, and for some
// representations of data leaf nodes (that don't use raw nodes or
// Filestore).
//
// It aims to replace the `UnixfsNode` structure which encapsulated too
// many possible node state combinations.
//
// TODO: Revisit the name.
type FSNodeOverDag struct {
	dag  *dag.ProtoNode
	file *ft.FSNode
}

// NewFSNodeOverDag creates a new `dag.ProtoNode` and `ft.FSNode`
// decoupled from one onther (and will continue in that way until
// `Commit` is called), with `fsNodeType` specifying the type of
// the UnixFS layer node (either `File` or `Raw`).
func (db *DagBuilderHelper) NewFSNodeOverDag(fsNodeType pb.Data_DataType) *FSNodeOverDag {
	node := new(FSNodeOverDag)
	node.dag = new(dag.ProtoNode)
	node.dag.SetCidBuilder(db.GetCidBuilder())

	node.file = ft.NewFSNode(fsNodeType)

	return node
}

// NewFSNFromDag reconstructs a FSNodeOverDag node from a given dag node
func (db *DagBuilderHelper) NewFSNFromDag(nd *dag.ProtoNode) (*FSNodeOverDag, error) {
	return NewFSNFromDag(nd)
}

// NewFSNFromDag reconstructs a FSNodeOverDag node from a given dag node
func NewFSNFromDag(nd *dag.ProtoNode) (*FSNodeOverDag, error) {
	mb, err := ft.FSNodeFromBytes(nd.Data())
	if err != nil {
		return nil, err
	}

	return &FSNodeOverDag{
		dag:  nd,
		file: mb,
	}, nil
}

// AddChild adds a `child` `ipld.Node` to both node layers. The
// `dag.ProtoNode` creates a link to the child node while the
// `ft.FSNode` stores its file size (that is, not the size of the
// node but the size of the file data that it is storing at the
// UnixFS layer). The child is also stored in the `DAGService`.
func (n *FSNodeOverDag) AddChild(child ipld.Node, fileSize uint64, db *DagBuilderHelper) error {
	err := n.dag.AddNodeLink("", child)
	if err != nil {
		return err
	}

	n.file.AddBlockSize(fileSize)

	return db.Add(child)
}

// RemoveChild deletes the child node at the given index.
func (n *FSNodeOverDag) RemoveChild(index int, dbh *DagBuilderHelper) {
	n.file.RemoveBlockSize(index)
	n.dag.SetLinks(append(n.dag.Links()[:index], n.dag.Links()[index+1:]...))
}

// Commit unifies (resolves) the cache nodes into a single `ipld.Node`
// that represents them: the `ft.FSNode` is encoded inside the
// `dag.ProtoNode`.
//
// TODO: Make it read-only after committing, allow to commit only once.
func (n *FSNodeOverDag) Commit() (ipld.Node, error) {
	fileData, err := n.file.GetBytes()
	if err != nil {
		return nil, err
	}
	n.dag.SetData(fileData)

	return n.dag, nil
}

// NumChildren returns the number of children of the `ft.FSNode`.
func (n *FSNodeOverDag) NumChildren() int {
	return n.file.NumChildren()
}

// FileSize returns the `Filesize` attribute from the underlying
// representation of the `ft.FSNode`.
func (n *FSNodeOverDag) FileSize() uint64 {
	return n.file.FileSize()
}

// SetFileData stores the `fileData` in the `ft.FSNode`. It
// should be used only when `FSNodeOverDag` represents a leaf
// node (internal nodes don't carry data, just file sizes).
func (n *FSNodeOverDag) SetFileData(fileData []byte) {
	n.file.SetData(fileData)
}

// GetDagNode fills out the proper formatting for the FSNodeOverDag node
// inside of a DAG node and returns the dag node.
// TODO: Check if we have committed (passed the UnixFS information
// to the DAG layer) before returning this.
func (n *FSNodeOverDag) GetDagNode() (ipld.Node, error) {
	return n.dag, nil
}

// GetChild gets the ith child of this node from the given DAGService.
func (n *FSNodeOverDag) GetChild(ctx context.Context, i int, ds ipld.DAGService) (*FSNodeOverDag, error) {
	nd, err := n.dag.Links()[i].GetNode(ctx, ds)
	if err != nil {
		return nil, err
	}

	pbn, ok := nd.(*dag.ProtoNode)
	if !ok {
		return nil, dag.ErrNotProtobuf
	}

	return NewFSNFromDag(pbn)
}
