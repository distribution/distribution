package s3

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
	"github.com/AdRoll/goamz/s3/s3test"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var s3DriverConstructor func(rootDirectory string) (*Driver, error)
var skipS3 func() string

func init() {
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	encrypt := os.Getenv("S3_ENCRYPT")
	secure := os.Getenv("S3_SECURE")
	v4auth := os.Getenv("S3_USE_V4_AUTH")
	region := os.Getenv("AWS_REGION")
	s3EntryPoint := os.Getenv("S3_ENTRYPOINT")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	s3DriverConstructor = func(rootDirectory string) (*Driver, error) {
		regionSupportHead := true

		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		secureBool := true
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		v4AuthBool := false
		if v4auth != "" {
			v4AuthBool, err = strconv.ParseBool(v4auth)
			if err != nil {
				return nil, err
			}
		}

		var realregion aws.Region
		if region == "generic" {
			if s3EntryPoint == "" {
				return nil, fmt.Errorf("No S3 endpoint for generic region")
			}
			realregion = aws.Region{Name: region, S3Endpoint: s3EntryPoint, S3LocationConstraint: true}
		} else {
			realregion = aws.GetRegion(region)
			if realregion.Name == "" {
				return nil, fmt.Errorf("Invalid region provided: %v", region)
			}
		}
		parameters := DriverParameters{
			accessKey,
			secretKey,
			bucket,
			realregion,
			regionSupportHead,
			encryptBool,
			secureBool,
			v4AuthBool,
			minChunkSize,
			rootDirectory,
		}

		return New(parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipS3 = func() string {
		if region == "generic" && (accessKey == "" || secretKey == "" || bucket == "" || s3EntryPoint == "") {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, S3_BUCKET, and S3_ENTRYPOINT to test RadosGW as storage"
		}
		if region == "generic" {
			return ""
		}
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(root)
	}, skipS3)
}

func TestEmptyRootList(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := s3DriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := s3DriverConstructor("")
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := s3DriverConstructor("/")
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer rootedDriver.Delete(ctx, filename)

	keys, err := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, err = slashRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

func startS3Server(bucketName string) (*s3test.Server, error) {
	srv, err := s3test.NewServer(nil)
	if err != nil {
		return nil, err
	}
	auth := aws.Auth{AccessKey: "accesskey", SecretKey: "secretkey"}
	region := aws.Region{Name: "generic", S3Endpoint: srv.URL(), S3LocationConstraint: true}
	conn := s3.New(auth, region)
	bucket := conn.Bucket(bucketName)
	bucket.PutBucket(s3.Private)
	return srv, nil
}

func TestFromParametersWithGenericRegion(t *testing.T) {
	bucketName := "bkt-name"
	srv, err := startS3Server(bucketName)
	if err != nil {
		t.Fatalf("unexpected error while setting S3 server mock")
	}
	params := map[string]interface{}{
		"accesskey":      "accesskey",
		"secretkey":      "secretkey",
		"region":         "generic",
		"regionendpoint": srv.URL(),
		"bucket":         bucketName,
	}
	d, err := FromParameters(params)
	if err != nil {
		t.Fatalf("unexpected error while creating driver: %q", err)
	}
	drv, _ := d.baseEmbed.Base.StorageDriver.(*driver)
	if bName := drv.Bucket.Name; bName != params["bucket"] {
		t.Fatalf("invalid bucket name: expected: %q got: %q", params["bucket"], bName)
	}
	if regionName := drv.S3.Name; regionName != params["region"] {
		t.Fatalf("invalid region name: expected: %q got: %q", params["region"], regionName)
	}
	if s3endpoint := drv.S3.S3Endpoint; s3endpoint != params["regionendpoint"] {
		t.Fatalf("invalid region endpoint: expected: %q got: %q", params["regionendpoint"], s3endpoint)
	}
	if s3locConstr := drv.S3.S3LocationConstraint; s3locConstr == false {
		t.Fatalf("S3 Location Constraint not properly set: expected: %t got: %t", true, s3locConstr)
	}
}

func TestFromParametersWithDisabledHeadRequests(t *testing.T) {
	bucketName := "bkt-name"
	srv, err := startS3Server(bucketName)
	if err != nil {
		t.Fatalf("unexpected error while setting S3 server mock")
	}
	params := map[string]interface{}{
		"accesskey":          "accesskey",
		"secretkey":          "secretkey",
		"region":             "generic",
		"regionendpoint":     srv.URL(),
		"regionsupportshead": false,
		"bucket":             bucketName,
	}
	d, err := FromParameters(params)
	if err != nil {
		t.Fatalf("unexpected error while creating driver: %q", err)
	}
	drv, _ := d.baseEmbed.Base.StorageDriver.(*driver)
	if supportsHead := drv.SupportsHead; supportsHead != false {
		t.Fatalf("not properly set: expected: %t got: %t", true, supportsHead)
	}
}

func TestFromParametersDefaultSupportsHead(t *testing.T) {
	bucketName := "bkt-name"
	srv, err := startS3Server(bucketName)
	if err != nil {
		t.Fatalf("unexpected error while setting S3 server mock")
	}
	params := map[string]interface{}{
		"accesskey":      "accesskey",
		"secretkey":      "secretkey",
		"region":         "generic",
		"regionendpoint": srv.URL(),
		"bucket":         bucketName,
	}
	d, err := FromParameters(params)
	if err != nil {
		t.Fatalf("unexpected error while creating driver: %q", err)
	}
	drv, _ := d.baseEmbed.Base.StorageDriver.(*driver)
	if supportsHead := drv.SupportsHead; supportsHead == false {
		t.Fatalf("Region HEAD requests disabled by default: expected: %t got: %t", true, supportsHead)
	}
}

func TestDriverURLForWithHeadDisabled(t *testing.T) {
	bucketName := "bkt-name"
	srv, err := startS3Server(bucketName)
	if err != nil {
		t.Fatalf("unexpected error while setting S3 server mock")
	}
	params := map[string]interface{}{
		"accesskey":          "accesskey",
		"secretkey":          "secretkey",
		"region":             "generic",
		"regionendpoint":     srv.URL(),
		"regionsupportshead": false,
		"bucket":             bucketName,
	}
	d, err := FromParameters(params)
	if err != nil {
		t.Fatalf("unexpected error while creating driver: %q", err)
	}
	if _, err := d.URLFor(nil, "/test", nil); err != storagedriver.ErrUnsupportedMethod {
		t.Fatalf("Invalid error type: expected: %q got: %q", storagedriver.ErrUnsupportedMethod, err)
	}
}
