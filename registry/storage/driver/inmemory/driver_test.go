package inmemory

import (
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"github.com/stretchr/testify/suite"
)

func newDriverSuite() *testsuites.DriverSuite {
	inmemoryDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(), nil
	}
	return testsuites.NewDriverSuite(inmemoryDriverConstructor, testsuites.NeverSkip)
}

func TestInMemoryDriverSuite(t *testing.T) {
	suite.Run(t, newDriverSuite())
}

func BenchmarkInMemoryDriverSuite(b *testing.B) {
	benchsuite := testsuites.NewDriverBenchmarkSuite(newDriverSuite())
	benchsuite.Suite.SetupSuite()
	b.Cleanup(benchsuite.Suite.TearDownSuite)

	b.Run("PutGetEmptyFiles", benchsuite.BenchmarkPutGetEmptyFiles)
	b.Run("PutGet1KBFiles", benchsuite.BenchmarkPutGet1KBFiles)
	b.Run("PutGet1MBFiles", benchsuite.BenchmarkPutGet1MBFiles)
	b.Run("PutGet1GBFiles", benchsuite.BenchmarkPutGet1GBFiles)
	b.Run("StreamEmptyFiles", benchsuite.BenchmarkStreamEmptyFiles)
	b.Run("Stream1KBFiles", benchsuite.BenchmarkStream1KBFiles)
	b.Run("Stream1MBFiles", benchsuite.BenchmarkStream1MBFiles)
	b.Run("Stream1GBFiles", benchsuite.BenchmarkStream1GBFiles)
	b.Run("List5Files", benchsuite.BenchmarkList5Files)
	b.Run("List50Files", benchsuite.BenchmarkList50Files)
	b.Run("Delete5Files", benchsuite.BenchmarkDelete5Files)
	b.Run("Delete50Files", benchsuite.BenchmarkDelete50Files)
}
