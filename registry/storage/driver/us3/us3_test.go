package us3

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var us3DriverConstructor func(rootDirectory string) (*Driver, error)

var skipCheck func() string

func init() {
	publicKey := os.Getenv("PUBLICKEY")
	privateKey := os.Getenv("PRIVATEKEY")
	api := os.Getenv("API")
	bucket := os.Getenv("BUCKET")
	regin := os.Getenv("REGIN")
	endpoint := os.Getenv("ENDPOINT")
	verifyUploadMD5Bool := os.Getenv("VERIFYUPLOADMD5")
	root, err := ioutil.TempDir("", "driver-") // ioutil.TempDir 会创建一个临时文件
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	us3DriverConstructor = func(rootDirectory string) (*Driver, error) {

		verifyUploadMD5 := false

		if verifyUploadMD5Bool != "" {
			verifyUploadMD5, err = strconv.ParseBool(verifyUploadMD5Bool)
			if err != nil {
				return nil, err
			}
		}

		param := DriverParameters{
			PublicKey:       publicKey,
			PrivateKey:      privateKey,
			Api:             api,
			Bucket:          bucket,
			Regin:           regin,
			Endpoint:        endpoint,
			VerifyUploadMD5: verifyUploadMD5,
			RootDirectory:   rootDirectory,
		}
		return New(param)
	}

	skipCheck = func() string {
		if publicKey == "" || privateKey == "" || bucket == "" || regin == "" || endpoint == "" {
			return "Must set PUBLICKEY, PRIVATEKEY, BUCKET, REGIN and ENDPOINT to run us3 tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return us3DriverConstructor(root)
	}, skipCheck)
}
