package ipfs

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	badger "github.com/ipfs/go-ds-badger"
	crdt "github.com/ipfs/go-ds-crdt"
	logging "github.com/ipfs/go-log"
	unixfs "github.com/ipfs/go-unixfs"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	multiaddr "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/ugorji/go/codec"
)

const driverName = "ipfs"

var (
	ipfsStoreNamespace = datastore.NewKey("/i")
	crdtStoreNamespace = datastore.NewKey("/c")
	privateKeyKey      = datastore.NewKey("/privkey")
	pubsubTopic        = "ipfs-registry"
)

var _ storagedriver.StorageDriver = (*Driver)(nil)

func init() {
	factory.Register(driverName, &ipfsDriverFactory{})

	_ = logging.SetLogLevel

	// Uncomment for debugging on libp2p stack
	// logging.SetLogLevel("pubsub", "debug")
	// logging.SetLogLevel("dht", "debug")
}

type ipfsDriverFactory struct{}

func (fact *ipfsDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	opts := DefaultOptions

	for k, v := range parameters {
		if v == nil || fmt.Sprint(v) == "" {
			continue
		}
		switch k {
		case "pubsubtopic":
			opts.PubSubTopic = fmt.Sprint(v)
		case "bootstrappers":
			list := strings.Split(fmt.Sprint(v), ",")
			infos := []peerstore.PeerInfo{}
			for _, l := range list {
				maddr, err := multiaddr.NewMultiaddr(l)
				if err != nil {
					return nil, fmt.Errorf("error parsing muiltiaddress: %s (%s)", l, err)
				}
				inf, err := peerstore.InfoFromP2pAddr(maddr)
				if err != nil {
					return nil, err
				}
				infos = append(infos, *inf)
			}
			opts.Bootstrappers = infos

		case "listenaddrs":
			list := strings.Split(fmt.Sprint(v), ",")
			maddrs := []multiaddr.Multiaddr{}
			for _, l := range list {
				maddr, err := multiaddr.NewMultiaddr(l)
				if err != nil {
					return nil, fmt.Errorf("error parsing muiltiaddress: %s (%s)", l, err)
				}
				maddrs = append(maddrs, maddr)
			}
			opts.ListenAddrs = maddrs

		case "protectorkey":
			secret, err := hex.DecodeString(fmt.Sprint(v))
			if err != nil {
				return nil, fmt.Errorf("error decoding protectorkey: %s", err)
			}
			if len(secret) != 32 {
				return nil, errors.New("secret must be 32 bytes")
			}
			opts.ProtectorKey = secret
		case "offline":
			switch v.(type) {
			case string:
				b, err := strconv.ParseBool(v.(string))
				if err != nil {
					return nil, errors.New("error parsing the offline parameter")
				}
				opts.Offline = b
			case bool:
				opts.Offline = v.(bool)
			}
		case "datastorepath":
			opts.DatastorePath = fmt.Sprint(v)
		case "privatekey":
			pkb, err := base64.StdEncoding.DecodeString(fmt.Sprint(v))
			if err != nil {
				return nil, fmt.Errorf("error decoding private key: %s", err)
			}
			pKey, err := crypto.UnmarshalPrivateKey(pkb)
			if err != nil {
				return nil, fmt.Errorf("error unmarshaling private key: %s", err)
			}
			opts.PrivateKey = pKey
		case "gatewayurl":
			opts.GatewayURL = fmt.Sprint(v)
		case "loglevel":
			v := fmt.Sprint(v)
			lvl, err := logrus.ParseLevel(v)
			if err != nil {
				return nil, err
			}
			opts.LogLevel = lvl
		}
	}

	return New(context.Background(), opts)
}

type Options struct {
	PubSubTopic   string
	Bootstrappers []peerstore.PeerInfo
	ListenAddrs   []multiaddr.Multiaddr
	ProtectorKey  []byte
	Offline       bool
	DatastorePath string
	PrivateKey    crypto.PrivKey
	GatewayURL    string
	LogLevel      logrus.Level
}

func mustParseMultiaddr(maStr string) multiaddr.Multiaddr {
	maddr, err := multiaddr.NewMultiaddr(maStr)
	if err != nil {
		panic(err)
	}
	return maddr
}

// DefaultOptions provides default values for the ipfs storage
// driver options.
var DefaultOptions Options = Options{
	PubSubTopic:   pubsubTopic,
	Bootstrappers: nil, // defaults will be used when ProtectorKey unset
	ListenAddrs:   []multiaddr.Multiaddr{mustParseMultiaddr("/ip4/0.0.0.0/tcp/4619")},
	ProtectorKey:  nil,
	Offline:       false,
	DatastorePath: "/var/lib/registry/ipfs",
	GatewayURL:    "http://localhost:5001",
	LogLevel:      logrus.ErrorLevel,
}

// baseEmbed allows us to hide the Base embed.
type baseEmbed struct {
	base.Base
}

// Driver implements the storagedriver.StorageDriver interface backed by
// an IPFS-lite embedded node. It uses a go-ds-crdt datastore to store paths
// and simulate a filesystem.
type Driver struct {
	baseEmbed
}

// New builds a new IPFS storage driver with the given options.
// If the PrivateKey is not set, a new key will be randomly generated
// and stored in the datastore.
func New(ctx context.Context, opts Options) (*Driver, error) {
	d, err := newDriver(ctx, opts)
	if err != nil {
		return nil, setDriverError("", err)
	}
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

type driver struct {
	options *Options

	ctx context.Context

	host         host.Host
	dht          *dht.IpfsDHT
	pubsub       *pubsub.PubSub
	subscription *pubsub.Subscription
	ipfs         *ipfslite.Peer
	baseStore    datastore.Datastore
	crdtStore    *crdt.Datastore
	ipfsStore    datastore.Datastore

	logger *logrus.Logger
}

// generates a random one if not found
func privKeyFromStore(store datastore.Datastore) (crypto.PrivKey, error) {
	privBytes, err := store.Get(privateKeyKey)

	if err == datastore.ErrNotFound {
		priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
		return priv, err
	}

	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(privBytes)
}

func newDriver(ctx context.Context, opts Options) (*driver, error) {
	logger := logrus.New()
	logger.Level = opts.LogLevel
	logger.Info("ipfs: initializing ipfs storage driver")

	if opts.ProtectorKey == nil && opts.Bootstrappers == nil {
		opts.Bootstrappers = ipfslite.DefaultBootstrapPeers()
	}

	// Disallow the use of the default pubsub topic when joining
	// the public network.
	if opts.PubSubTopic == pubsubTopic && opts.ProtectorKey == nil {
		return nil, errors.New("set a custom pubsubtopic or a protectorkey")
	}

	baseStore, err := badger.NewDatastore(opts.DatastorePath, &badger.DefaultOptions)
	if err != nil {
		return nil, err
	}

	logger.Infof("ipfs: loaded datastore at %s", opts.DatastorePath)

	// Read existing key from datastore or generate new one
	if opts.PrivateKey == nil {
		priv, err := privKeyFromStore(baseStore)
		if err != nil {
			return nil, err
		}
		opts.PrivateKey = priv
	}

	pid, err := peer.IDFromPublicKey(opts.PrivateKey.GetPublic())
	if err != nil {
		return nil, err
	}
	logger.Infof("ipfs: IPFS peer ID: %s", peer.IDB58Encode(pid))

	// Write key to the store
	privBytes, err := crypto.MarshalPrivateKey(opts.PrivateKey)
	if err != nil {
		return nil, err
	}
	err = baseStore.Put(privateKeyKey, privBytes)
	if err != nil {
		return nil, err
	}

	ipfsStore := namespace.Wrap(baseStore, ipfsStoreNamespace)

	logger.Info("ipfs: initializing libp2p host and DHT")

	h, dht, err := ipfslite.SetupLibp2p(
		ctx,
		opts.PrivateKey,
		opts.ProtectorKey,
		opts.ListenAddrs,
	)

	if err != nil {
		baseStore.Close()
		return nil, err
	}

	logger.Info("ipfs: libp2p peer, dht created. Peer addresses:")
	for _, l := range h.Addrs() {
		logger.Infof("  - %s/ipfs/%s", l, h.ID())
	}
	logger.Info("ipfs: use the above addresses to bootstrap other peers")

	psub, err := pubsub.NewGossipSub(
		ctx,
		h,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		h.Close()
		baseStore.Close()
		return nil, err
	}
	logger.Infof("ipfs: pubsub created. Topic: %s", opts.PubSubTopic)

	ipfs, err := ipfslite.New(
		ctx,
		ipfsStore,
		h,
		dht,
		&ipfslite.Config{
			Offline: false,
		},
	)
	if err != nil {
		h.Close()
		baseStore.Close()
		return nil, err
	}

	logger.Info("ipfs: IPFS-Lite peer initialized. Bootstraping to:")
	for _, b := range opts.Bootstrappers {
		logger.Infof("  - %s", b)
	}
	ipfs.Bootstrap(opts.Bootstrappers)

	bcast, err := crdt.NewPubSubBroadcaster(ctx, psub, opts.PubSubTopic)
	if err != nil {
		h.Close()
		baseStore.Close()
		return nil, err
	}

	crdtopts := crdt.DefaultOptions()
	crdtopts.Logger = logger

	crdtStore, err := crdt.New(
		baseStore,
		crdtStoreNamespace,
		ipfs,
		bcast,
		crdtopts,
	)
	if err != nil {
		h.Close()
		baseStore.Close()
		return nil, err
	}

	logger.Info("ipfs: replicated CRDT store intialized")

	go func() {
		select {
		case <-ctx.Done():
			h.Close()
			baseStore.Close()
		}
	}()

	logger.Info("ipfs: storage driver ready to use")

	drv := &driver{
		ctx:       ctx,
		options:   &opts,
		baseStore: baseStore,
		ipfsStore: ipfsStore,
		crdtStore: crdtStore,
		host:      h,
		dht:       dht,
		pubsub:    psub,
		ipfs:      ipfs,
		logger:    logger,
	}
	return drv, nil

}

// Name returns the driver name.
func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	rsc, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	defer rsc.Close()
	full, err := ioutil.ReadAll(rsc)
	if err != nil {
		return nil, err
	}
	return full, nil
}

// This should primarily be used for small objects.
func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	w, err := d.Writer(ctx, path, false)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(content)
	if err != nil {
		return err
	}
	return w.Commit()
}

func setDriverError(path string, err error) error {
	if err == nil {
		return nil
	}
	if err == datastore.ErrNotFound {
		return storagedriver.PathNotFoundError{path, driverName}
	}
	return storagedriver.Error{driverName, err}
}

func (d *driver) getFileInfo(path string) (*fileInfo, error) {
	fiBytes, err := d.crdtStore.Get(datastore.NewKey(path))
	if err != nil {
		return nil, setDriverError(path, err)
	}

	return fileInfoUnmarshal(fiBytes)
}

// Reader retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	fi, err := d.getFileInfo(path)
	if err != nil {
		return nil, err
	}

	rsc, err := d.ipfs.GetFile(ctx, fi.FCid)
	if err != nil {
		return nil, setDriverError(path, err)
	}

	if offset > 0 {
		_, err = rsc.Seek(offset, io.SeekStart)
		if err != nil {
			rsc.Close()
			return nil, storagedriver.InvalidOffsetError{path, offset, driverName}
		}
	}
	return rsc, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	fw := newFileWriter(ctx, path, d.ipfs, d.crdtStore)
	if append {
		// Read existing file and write it again
		r, err := d.Reader(ctx, path, 0)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		_, err = io.Copy(fw, r)
		if err != nil {
			return nil, setDriverError(path, err)
		}
	}
	return fw, nil
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	fi, err := d.getFileInfo(path)

	// if found just return it.
	if err == nil {
		fi.FPath = path
		return fi, nil
	}

	if _, ok := err.(storagedriver.PathNotFoundError); !ok {
		return nil, err
	}

	// if we are here, we got a PathNotFound error.
	results, err := d.crdtStore.Query(query.Query{
		Prefix: path,
	})
	if err != nil {
		return nil, setDriverError(path, err)
	}
	defer results.Close()
	all, err := results.Rest()
	if err != nil {
		return nil, setDriverError(path, err)
	}
	if len(all) == 0 && path != "/" { // nothing in this folder, then it does not exist
		return nil, setDriverError(path, datastore.ErrNotFound)
	}

	folderFileInfo := storagedriver.FileInfoInternal{
		FileInfoFields: storagedriver.FileInfoFields{
			Path:  path,
			Size:  0,
			IsDir: true,
		},
	}

	// Set modTime to that of the latest.
	for _, r := range all {
		fi, err := fileInfoUnmarshal(r.Value)
		if err != nil {
			return nil, setDriverError(path, err)
		}
		if fi.FModTime.After(folderFileInfo.ModTime()) {
			folderFileInfo.FileInfoFields.ModTime = fi.FModTime
		}
	}
	return folderFileInfo, nil
}

type directDescendantFilter struct {
	basePath datastore.Key
}

func (ddf *directDescendantFilter) Filter(e query.Entry) bool {
	cur := datastore.NewKey(e.Key)
	return cur.IsDescendantOf(ddf.basePath)
}

// List returns a list of the objects that are direct descendants of the
//given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	results, err := d.crdtStore.Query(query.Query{
		Prefix:   path,
		KeysOnly: true,
		Filters:  []query.Filter{&directDescendantFilter{datastore.NewKey(path)}},
	})
	if err != nil {
		return nil, setDriverError(path, err)
	}
	defer results.Close()
	paths := []string{}

	pathsMap := make(map[string]struct{})

	for r := range results.Next() {
		if r.Error != nil {
			return nil, setDriverError(path, r.Error)
		}

		basePath := datastore.NewKey(path)
		for k := datastore.NewKey(r.Key); k.IsDescendantOf(basePath); k = k.Parent() {
			if k.Parent().Equal(basePath) {
				pathsMap[k.String()] = struct{}{}
			}
		}
	}

	for k := range pathsMap {
		paths = append(paths, k)
	}

	if len(paths) == 0 && path != "/" {
		return nil, setDriverError(path, datastore.ErrNotFound)
	}
	return paths, nil
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
// Note: This may be no more efficient than a copy followed by a delete for
// many implementations.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	// store CID in new path and delete the old
	v, err := d.crdtStore.Get(datastore.NewKey(sourcePath))
	if err != nil {
		return setDriverError(sourcePath, err)
	}
	err = d.crdtStore.Put(datastore.NewKey(destPath), v)
	if err != nil {
		return setDriverError(destPath, err)
	}
	return setDriverError(sourcePath, d.Delete(ctx, sourcePath))
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	// there is no delete in the permanent web
	return setDriverError(path, d.crdtStore.Delete(datastore.NewKey(path)))
}

// URLFor returns the IPFS gateway URL for the given path.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod{driverName}
	// v, err := d.getFileInfo(path)
	// if err != nil {
	// 	return "", err
	// }
	// return fmt.Sprintf("%s/ipfs/%s", d.options.GatewayURL, v.FCid), nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file.
// If the returned error from the WalkFn is ErrSkipDir and fileInfo refers
// to a directory, the directory will not be entered and Walk
// will continue the traversal.  If fileInfo refers to a normal file, processing stops
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

type fileWriter struct {
	cancel context.CancelFunc
	path   string

	r *io.PipeReader
	w *io.PipeWriter

	commitCh chan struct{}
	errorCh  chan error

	size int64
}

func newFileWriter(ctx context.Context, path string, ipfs *ipfslite.Peer, crdt datastore.Datastore) *fileWriter {
	ctx, cancel := context.WithCancel(ctx)

	r, w := io.Pipe()

	commitCh := make(chan struct{}, 1)
	errorCh := make(chan error, 1)
	go func() {
		defer close(errorCh)
		nd, err := ipfs.AddFile(ctx, r, nil)
		if err != nil {
			errorCh <- err
			return
		}
		<-commitCh
		ufsnd, err := unixfs.ExtractFSNode(nd)
		if err != nil {
			errorCh <- err
			return
		}
		size := ufsnd.FileSize()

		fi := &fileInfo{
			FCid:     nd.Cid(),
			FSize:    int64(size),
			FModTime: time.Now(),
		}

		bs, err := fi.Marshal()
		if err != nil {
			errorCh <- err
			return
		}

		err = crdt.Put(datastore.NewKey(path), bs)
		if err != nil {
			errorCh <- err
		}
	}()

	return &fileWriter{
		cancel:   cancel,
		r:        r,
		w:        w,
		commitCh: commitCh,
		errorCh:  errorCh,
		size:     0,
	}
}

func (fw *fileWriter) Close() error {
	return fw.Commit()
}

func (fw *fileWriter) Cancel() error {
	fw.cancel()
	return fw.w.CloseWithError(errors.New("cancelled"))
}

func (fw *fileWriter) Write(b []byte) (int, error) {
	n, err := fw.w.Write(b)
	atomic.AddInt64(&fw.size, int64(n))
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return atomic.LoadInt64(&fw.size)
}

func (fw *fileWriter) Commit() error {
	defer fw.cancel()
	err := fw.w.Close()
	if err != nil {
		return err
	}
	fw.commitCh <- struct{}{}
	err, ok := <-fw.errorCh
	if !ok {
		return nil
	}
	return err
}

var fiHandle = &codec.MsgpackHandle{}

// fileinfo is the information we store
// in the datastore.
type fileInfo struct {
	FPath    string    `codec:"-"`
	FCid     cid.Cid   `codec:"c"`
	FSize    int64     `codec:"s"`
	FModTime time.Time `codec:"t"`
}

func fileInfoUnmarshal(bs []byte) (*fileInfo, error) {
	dec := codec.NewDecoderBytes(bs, fiHandle)
	var fi fileInfo
	err := dec.Decode(&fi)
	return &fi, err
}

func (fi *fileInfo) Path() string {
	return fi.FPath
}

func (fi *fileInfo) Size() int64 {
	return fi.FSize
}

func (fi *fileInfo) ModTime() time.Time {
	return fi.FModTime
}

// a file is never a dir.
func (fi *fileInfo) IsDir() bool {
	return false
}

func (fi *fileInfo) Marshal() ([]byte, error) {
	var b []byte
	enc := codec.NewEncoderBytes(&b, fiHandle)
	err := enc.Encode(fi)
	return b, err
}
