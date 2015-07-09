package speedy

import (
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
	"os"
	"strconv"
	"testing"
)

func Test(t *testing.T) { check.TestingT(t) }

func init() {
	storageURLs := os.Getenv("SPEEDY_STORAGE_URL")
	chunkSizeStr := os.Getenv("SPEEDY_CHUNKSIZE")
	heartBeatIntervalStr := os.Getenv("SPEEDY_HEART_BEAT")
	var chunkSize int         //MB
	var heartBeatInterval int //seconds

	if chunkSizeStr != "" {
		chunkSize, _ = strconv.Atoi(chunkSizeStr)
	}

	if heartBeatIntervalStr != "" {
		heartBeatInterval, _ = strconv.Atoi(heartBeatIntervalStr)
	}

	driverConstructor := func() (storagedriver.StorageDriver, error) {
		parameters := DriverParameters{
			storageURLs:       storageURLs,
			chunkSize:         uint64(chunkSize),
			heartBeatInterval: heartBeatInterval,
		}

		return New(parameters)
	}

	skipCheck := func() string {
		if storageURLs == "" || chunkSize == 0 || heartBeatInterval == 0 {
			return "Must set SPEEDY_STORAGE_URL, SPEEDY_CHUNKSIZE and SPEEDY_HEART_BEAT to run speedy test"
		}
		return ""
	}

	testsuites.RegisterSuite(driverConstructor, skipCheck)
}
