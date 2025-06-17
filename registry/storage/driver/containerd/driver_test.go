package containerd

import (
	"context"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"bytes"
	"io"

	"github.com/stretchr/testify/require"
)

func newTestDriver(t *testing.T) storagedriver.StorageDriver {
	t.Helper()
	parameters := map[string]interface{}{}
	driver, err := factory.Create(context.Background(), "containerd", parameters)
	require.NoError(t, err)
	require.NotNil(t, driver)
	return driver
}

func TestCreateDriver(t *testing.T) {
	driver := newTestDriver(t)
	require.Equal(t, "containerd", driver.Name())
}

func TestPushAndPull(t *testing.T) {
	ctx := context.Background()
	driver := newTestDriver(t)

	// Test pushing a small blob
	content1 := []byte("Hello, containerd!")
	path1 := "test/blob1"
	writer, err := driver.Writer(ctx, path1, false)
	require.NoError(t, err)
	_, err = io.Copy(writer, bytes.NewReader(content1))
	require.NoError(t, err)
	err = writer.Commit()
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)


	// Test pulling the blob
	reader, err := driver.Reader(ctx, path1, 0)
	require.NoError(t, err)
	defer reader.Close()
	retrievedContent1, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, content1, retrievedContent1)

	// Test pushing a larger blob
	content2 := bytes.Repeat([]byte("a"), 1024*1024) // 1MB
	path2 := "test/blob2"
	writer2, err := driver.Writer(ctx, path2, false)
	require.NoError(t, err)
	_, err = io.Copy(writer2, bytes.NewReader(content2))
	require.NoError(t, err)
	err = writer2.Commit()
	require.NoError(t, err)
	err = writer2.Close()
	require.NoError(t, err)

	// Test pulling the larger blob
	reader2, err := driver.Reader(ctx, path2, 0)
	require.NoError(t, err)
	defer reader2.Close()
	retrievedContent2, err := io.ReadAll(reader2)
	require.NoError(t, err)
	require.Equal(t, content2, retrievedContent2)

	// TODO: Add more test cases:
	// - Pushing with append=true (if supported)
	// - Pulling with an offset
	// - Pulling non-existent blob
	// - Pushing to an existing path (overwrite)
}


// Run the upstream test suite for inmemory driver
func TestContainerdDriverSuite(t *testing.T) {
	testsuites.Driver(t, func(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
		return factory.Create(ctx, "containerd", parameters)
	})
}
