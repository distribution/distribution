package mongodb

import (
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
	"os"
	"strings"
	"testing"
)

const (
	envMongodbURL = "MONGODB_STORAGE_URL"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	var (
		mongodbURL string
	)

	config := []struct {
		env   string
		value *string
	}{
		{envMongodbURL, &mongodbURL},
	}

	missing := []string{}
	for _, v := range config {
		*v.value = os.Getenv(v.env)
		if *v.value == "" {
			missing = append(missing, v.env)
		}
	}

	mongodbDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(mongodbURL, "docker_registry_test", nil)
	}

	// Skip MongoDB storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if len(missing) > 0 {
			return fmt.Sprintf("Must set %s environment variables to run MongoDB tests", strings.Join(missing, ", "))
		}
		return ""
	}

	testsuites.RegisterSuite(mongodbDriverConstructor, skipCheck)
}
