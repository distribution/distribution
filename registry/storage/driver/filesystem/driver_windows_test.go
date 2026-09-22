//go:build windows

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupMoveTest creates a driver with an existing source and destination file
// and returns the driver along with the files' paths on disk.
func setupMoveTest(t *testing.T) (d *Driver, srcFile, dstFile string) {
	root := t.TempDir()
	d, err := FromParameters(map[string]any{
		"rootdirectory": root,
	})
	require.NoError(t, err)

	srcFile = filepath.Join(root, "src.txt")
	dstFile = filepath.Join(root, "dst.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("old content"), 0o644))
	return d, srcFile, dstFile
}

// TestMoveLockedSourceKeepsDestination checks that when the source is held
// open elsewhere, Move fails and the existing destination is left intact.
func TestMoveLockedSourceKeepsDestination(t *testing.T) {
	d, srcFile, dstFile := setupMoveTest(t)

	// Go opens files without FILE_SHARE_DELETE, so this handle blocks
	// renaming the source.
	f, err := os.Open(srcFile)
	require.NoError(t, err)
	defer f.Close()

	require.Error(t, d.Move(context.Background(), "/src.txt", "/dst.txt"))

	got, err := os.ReadFile(dstFile)
	require.NoError(t, err, "destination must survive a failed move")
	require.Equal(t, []byte("old content"), got)
}

// TestMoveLockedDestinationKeepsDestination checks that when the destination
// is held open (e.g. by a concurrent reader), Move fails and the destination
// keeps its old content.
func TestMoveLockedDestinationKeepsDestination(t *testing.T) {
	d, srcFile, dstFile := setupMoveTest(t)

	f, err := os.Open(dstFile)
	require.NoError(t, err)
	defer f.Close()

	require.Error(t, d.Move(context.Background(), "/src.txt", "/dst.txt"))

	got, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	require.Equal(t, []byte("old content"), got)
	require.FileExists(t, srcFile)
}
