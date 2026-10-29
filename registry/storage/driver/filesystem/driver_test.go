package filesystem

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"github.com/stretchr/testify/require"
)

func newDriverConstructor(tb testing.TB) testsuites.DriverConstructor {
	root := tb.TempDir()

	return func() (storagedriver.StorageDriver, error) {
		return FromParameters(map[string]any{
			"rootdirectory": root,
		})
	}
}

func TestFilesystemDriverSuite(t *testing.T) {
	testsuites.Driver(t, newDriverConstructor(t), false)
}

func TestPutContent(t *testing.T) {
	root := t.TempDir()
	d := New(DriverParameters{RootDirectory: root, MaxThreads: minThreads})
	ctx := context.Background()
	const key = "/uploads/id/startedat"

	for _, content := range []string{"initial content", "shorter", ""} {
		require.NoError(t, d.PutContent(ctx, key, []byte(content)))
		got, err := d.GetContent(ctx, key)
		require.NoError(t, err)
		require.Equal(t, content, string(got))

		entries, err := os.ReadDir(filepath.Join(root, "uploads", "id"))
		require.NoError(t, err)
		require.Len(t, entries, 1, "temporary files must not be left behind")
		require.Equal(t, "startedat", entries[0].Name())
	}
}

func TestPutContentMoveFailure(t *testing.T) {
	root := t.TempDir()
	d := New(DriverParameters{RootDirectory: root, MaxThreads: minThreads})
	ctx := context.Background()

	// A nonempty directory cannot be replaced by the temporary file.
	dest := filepath.Join(root, "target")
	require.NoError(t, os.Mkdir(dest, 0o755))
	marker := filepath.Join(dest, "keep")
	require.NoError(t, os.WriteFile(marker, []byte("original"), 0o644))

	require.Error(t, d.PutContent(ctx, "/target", []byte("replacement")))
	got, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "original", string(got))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "failed renames must not leave temporary files")
	require.Equal(t, "target", entries[0].Name())
}

func TestPathNotFoundErrorIncludesRootDirectory(t *testing.T) {
	root := t.TempDir()
	d := New(DriverParameters{
		RootDirectory: root,
		MaxThreads:    minThreads,
	})

	missingPath := "/docker/registry/v2/blobs"
	_, err := d.Stat(context.Background(), missingPath)
	if err == nil {
		t.Fatal("expected error stating missing path")
	}

	pathNotFound, ok := err.(storagedriver.PathNotFoundError)
	if !ok {
		t.Fatalf("expected PathNotFoundError, got %T: %v", err, err)
	}

	errStr := pathNotFound.Error()
	if !strings.Contains(errStr, root) {
		t.Fatalf("expected error to contain rootdirectory %q, got %q", root, errStr)
	}
	if !strings.Contains(errStr, missingPath) {
		t.Fatalf("expected error to contain storage path %q, got %q", missingPath, errStr)
	}
}

func TestWriterBufferSize(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		size     uint64
		expected int
	}{
		{name: "unset", size: 0, expected: int(defaultWriteBufferSize)},
		{name: "configured", size: 64 * 1024, expected: 64 * 1024},
		{name: "too large", size: math.MaxUint64, expected: int(defaultWriteBufferSize)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := New(DriverParameters{
				RootDirectory:   t.TempDir(),
				MaxThreads:      minThreads,
				WriteBufferSize: tc.size,
			})

			w, err := d.Writer(ctx, "/file", false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, w.(*fileWriter).bw.Size())

			// Write more than one buffer's worth of data in small chunks so
			// that the buffer is both filled and flushed.
			contents := bytes.Repeat([]byte("0123456789abcdef"), tc.expected/8+1)
			for chunk := range slices.Chunk(contents, 1000) {
				_, err := w.Write(chunk)
				require.NoError(t, err)
			}
			require.Equal(t, int64(len(contents)), w.Size())
			require.NoError(t, w.Commit(ctx))
			require.NoError(t, w.Close())

			got, err := d.GetContent(ctx, "/file")
			require.NoError(t, err)
			require.Equal(t, contents, got)
		})
	}
}

func BenchmarkFilesystemDriverSuite(b *testing.B) {
	testsuites.BenchDriver(b, newDriverConstructor(b))
}

func TestFromParametersImpl(t *testing.T) {
	tests := []struct {
		params   map[string]any // technically the yaml can contain anything
		expected DriverParameters
		pass     bool
	}{
		// check we use default threads and root dirs
		{
			params: map[string]any{},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      defaultMaxThreads,
				WriteBufferSize: defaultWriteBufferSize,
			},
			pass: true,
		},
		// Testing initiation with a string maxThreads which can't be parsed
		{
			params: map[string]any{
				"maxthreads": "fail",
			},
			expected: DriverParameters{},
			pass:     false,
		},
		{
			params: map[string]any{
				"maxthreads": "100",
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      uint64(100),
				WriteBufferSize: defaultWriteBufferSize,
			},
			pass: true,
		},
		{
			params: map[string]any{
				"maxthreads": 100,
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      uint64(100),
				WriteBufferSize: defaultWriteBufferSize,
			},
			pass: true,
		},
		// check that we use minimum thread counts
		{
			params: map[string]any{
				"maxthreads": 1,
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      minThreads,
				WriteBufferSize: defaultWriteBufferSize,
			},
			pass: true,
		},
		{
			params: map[string]any{
				"writebuffersize": "1048576",
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      defaultMaxThreads,
				WriteBufferSize: uint64(1048576),
			},
			pass: true,
		},
		{
			params: map[string]any{
				"writebuffersize": 1048576,
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      defaultMaxThreads,
				WriteBufferSize: uint64(1048576),
			},
			pass: true,
		},
		// check that we use the minimum write buffer size
		{
			params: map[string]any{
				"writebuffersize": 1,
			},
			expected: DriverParameters{
				RootDirectory:   defaultRootDirectory,
				MaxThreads:      defaultMaxThreads,
				WriteBufferSize: defaultWriteBufferSize,
			},
			pass: true,
		},
		// Testing initiation with a string writebuffersize which can't be parsed
		{
			params: map[string]any{
				"writebuffersize": "fail",
			},
			expected: DriverParameters{},
			pass:     false,
		},
		{
			params: map[string]any{
				"writebuffersize": "18446744073709551615",
			},
			expected: DriverParameters{},
			pass:     false,
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
