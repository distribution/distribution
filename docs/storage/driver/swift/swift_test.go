package swift

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/ncw/swift/swifttest"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type SwiftDriverConstructor func(rootDirectory string) (*Driver, error)

func init() {
	var (
		username           string
		password           string
		authURL            string
		tenant             string
		tenantID           string
		domain             string
		domainID           string
		container          string
		region             string
		prefix             string
		insecureSkipVerify bool
		swiftServer        *swifttest.SwiftServer
		err                error
	)
	if username = os.Getenv("OS_USERNAME"); username == "" {
		username = os.Getenv("ST_USER")
	}
	if password = os.Getenv("OS_PASSWORD"); password == "" {
		password = os.Getenv("ST_KEY")
	}
	if authURL = os.Getenv("OS_AUTH_URL"); authURL == "" {
		authURL = os.Getenv("ST_AUTH")
	}
	tenant = os.Getenv("OS_TENANT_NAME")
	tenantID = os.Getenv("OS_TENANT_ID")
	domain = os.Getenv("OS_DOMAIN_NAME")
	domainID = os.Getenv("OS_DOMAIN_ID")
	container = os.Getenv("OS_CONTAINER_NAME")
	region = os.Getenv("OS_REGION_NAME")
	prefix = os.Getenv("OS_CONTAINER_PREFIX")
	insecureSkipVerify, _ = strconv.ParseBool(os.Getenv("ST_INSECURESKIPVERIFY"))

	if username == "" || password == "" || authURL == "" || container == "" {
		if swiftServer, err = swifttest.NewSwiftServer("localhost"); err != nil {
			panic(err)
		}
		username = "swifttest"
		password = "swifttest"
		authURL = swiftServer.AuthURL
		container = "test"
	}

	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	swiftDriverConstructor := func(rootDirectory string) (*Driver, error) {
		parameters := Parameters{
			username,
			password,
			authURL,
			tenant,
			tenantID,
			domain,
			domainID,
			region,
			container,
			prefix,
			insecureSkipVerify,
			defaultChunkSize,
		}

		return New(parameters)
	}

	skipCheck := func() string {
		return ""
	}

	driverConstructor := func() (storagedriver.StorageDriver, error) {
		return swiftDriverConstructor(root)
	}

	testsuites.RegisterInProcessSuite(driverConstructor, skipCheck)

	RegisterSwiftDriverSuite(swiftDriverConstructor, skipCheck, swiftServer)
}

func RegisterSwiftDriverSuite(swiftDriverConstructor SwiftDriverConstructor, skipCheck testsuites.SkipCheck,
	swiftServer *swifttest.SwiftServer) {
	check.Suite(&SwiftDriverSuite{
		Constructor: swiftDriverConstructor,
		SkipCheck:   skipCheck,
		SwiftServer: swiftServer,
	})
}

type SwiftDriverSuite struct {
	Constructor SwiftDriverConstructor
	SwiftServer *swifttest.SwiftServer
	testsuites.SkipCheck
}

func (suite *SwiftDriverSuite) SetUpSuite(c *check.C) {
	if reason := suite.SkipCheck(); reason != "" {
		c.Skip(reason)
	}
}

func (suite *SwiftDriverSuite) TestEmptyRootList(c *check.C) {
	validRoot, err := ioutil.TempDir("", "driver-")
	c.Assert(err, check.IsNil)
	defer os.Remove(validRoot)

	rootedDriver, err := suite.Constructor(validRoot)
	c.Assert(err, check.IsNil)
	emptyRootDriver, err := suite.Constructor("")
	c.Assert(err, check.IsNil)
	slashRootDriver, err := suite.Constructor("/")
	c.Assert(err, check.IsNil)

	filename := "/test"
	contents := []byte("contents")
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	c.Assert(err, check.IsNil)
	defer rootedDriver.Delete(ctx, filename)

	keys, err := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}

	keys, err = slashRootDriver.List(ctx, "/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}
}
