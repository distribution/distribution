// +build include_b2

package b2

import (
	"context"
	"os"
	"testing"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var skipB2 func() string

func init() {
	bucket := os.Getenv("B2_BUCKET")
	b2id := os.Getenv("B2_ACCOUNT_ID")
	b2key := os.Getenv("B2_SECRET_KEY")

	// Skip b2 storage driver tests if environment variable parameters are not provided
	skipB2 = func() string {
		if bucket == "" || b2id == "" || b2key == "" {
			return "The following environment variables must be set to enable these tests: B2_BUCKET, B2_ACCOUNT_ID, B2_SECRET_KEY"
		}
		return ""
	}

	if skipB2() != "" {
		return
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		f := &b2Factory{}
		return f.Create(map[string]interface{}{
			"bucket":              bucket,
			"id":                  b2id,
			"key":                 b2key,
			"context":             context.Background(),
			"concurrentuploads":   5,
			"concurrentdownloads": 5,
			"uploadchunksize":     1e8,
			"downloadchunksize":   5e7,
			"rootdirectory":       "/docker/tests",
		})
	}, skipB2)
}
