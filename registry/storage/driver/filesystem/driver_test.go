package filesystem

import (
	"reflect"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"github.com/stretchr/testify/suite"
)

func newDriverSuite(tb testing.TB) *testsuites.DriverSuite {
	root := tb.TempDir()

	drvr, err := FromParameters(map[string]interface{}{
		"rootdirectory": root,
	})
	if err != nil {
		panic(err)
	}

	return testsuites.NewDriverSuite(func() (storagedriver.StorageDriver, error) {
		return drvr, nil
	}, testsuites.NeverSkip)
}

func TestFilesystemDriverSuite(t *testing.T) {
	suite.Run(t, newDriverSuite(t))
}

func BenchmarkFilesystemDriverSuite(b *testing.B) {
	benchsuite := testsuites.NewDriverBenchmarkSuite(newDriverSuite(b))
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

func TestFromParametersImpl(t *testing.T) {
	tests := []struct {
		params   map[string]interface{} // technically the yaml can contain anything
		expected DriverParameters
		pass     bool
	}{
		// check we use default threads and root dirs
		{
			params: map[string]interface{}{},
			expected: DriverParameters{
				RootDirectory: defaultRootDirectory,
				MaxThreads:    defaultMaxThreads,
			},
			pass: true,
		},
		// Testing initiation with a string maxThreads which can't be parsed
		{
			params: map[string]interface{}{
				"maxthreads": "fail",
			},
			expected: DriverParameters{},
			pass:     false,
		},
		{
			params: map[string]interface{}{
				"maxthreads": "100",
			},
			expected: DriverParameters{
				RootDirectory: defaultRootDirectory,
				MaxThreads:    uint64(100),
			},
			pass: true,
		},
		{
			params: map[string]interface{}{
				"maxthreads": 100,
			},
			expected: DriverParameters{
				RootDirectory: defaultRootDirectory,
				MaxThreads:    uint64(100),
			},
			pass: true,
		},
		// check that we use minimum thread counts
		{
			params: map[string]interface{}{
				"maxthreads": 1,
			},
			expected: DriverParameters{
				RootDirectory: defaultRootDirectory,
				MaxThreads:    minThreads,
			},
			pass: true,
		},
	}

	for _, item := range tests {
		params, err := fromParametersImpl(item.params)

		if !item.pass {
			// We only need to assert that expected failures have an error
			if err == nil {
				t.Fatalf("expected error configuring filesystem driver with invalid param: %+v", item.params)
			}
			continue
		}

		if err != nil {
			t.Fatalf("unexpected error creating filesystem driver: %s", err)
		}
		// Note that we get a pointer to params back
		if !reflect.DeepEqual(*params, item.expected) {
			t.Fatalf("unexpected params from filesystem driver. expected %+v, got %+v", item.expected, params)
		}
	}
}
