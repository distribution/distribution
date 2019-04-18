package ipfs

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	peerstore "github.com/libp2p/go-libp2p-peerstore"

	"github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
)

var datastorePath string
var ctx, cancel = context.WithCancel(context.Background())

func Test(t *testing.T) {
	check.TestingT(t)
	defer os.RemoveAll(datastorePath)
	defer cancel()
}

func init() {
	path, err := ioutil.TempDir("", "ipfsdriver-")
	if err != nil {
		panic(err)
	}
	datastorePath = path

	opts := DefaultOptions
	opts.DatastorePath = datastorePath
	opts.Bootstrappers = []peerstore.PeerInfo{}
	opts.PubSubTopic = "test"
	opts.LogLevel = logrus.ErrorLevel

	ipfsDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(ctx, opts)
	}

	testsuites.RegisterSuite(ipfsDriverConstructor, testsuites.NeverSkip)
}
