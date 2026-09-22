//go:build windows

package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/internal/uuid"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/stretchr/testify/require"
)

func newTestDriver(t *testing.T) *Driver {
	d, err := FromParameters(map[string]any{
		"rootdirectory": t.TempDir(),
	})
	require.NoError(t, err)
	return d
}

// TestProof_WindowsOSRenameFailsOnExistingDirectory proves Claim 1:
// On Windows, Go's standard os.Rename fails when the destination is an existing
// directory (Win32 MoveFileExW returns ERROR_ACCESS_DENIED or ERROR_ALREADY_EXISTS),
// whereas the driver's fallback (os.Stat -> os.RemoveAll -> os.Rename) succeeds.
func TestProof_WindowsOSRenameFailsOnExistingDirectory(t *testing.T) {
	tempDir := t.TempDir()

	srcDir := filepath.Join(tempDir, "src_dir")
	dstDir := filepath.Join(tempDir, "dst_dir")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("src content"), 0o644))

	require.NoError(t, os.MkdirAll(dstDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "old.txt"), []byte("old content"), 0o644))

	// Proof Part A: Standard os.Rename fails on Windows when replacing an existing directory
	errStandardRename := os.Rename(srcDir, dstDir)
	require.Error(t, errStandardRename, "Standard os.Rename must fail on Windows when destination is an existing directory")
	t.Logf("[PROOF] Standard os.Rename failed as expected on Windows: %v", errStandardRename)

	// Verify old destination contents were not touched by the failed os.Rename
	require.FileExists(t, filepath.Join(dstDir, "old.txt"))

	// Proof Part B: The driver's rename() fallback succeeds by removing dest before retrying
	errFallbackRename := rename(srcDir, dstDir)
	require.NoError(t, errFallbackRename, "Fallback rename must succeed on Windows by removing existing destination first")
	t.Logf("[PROOF] Fallback rename succeeded replacing existing directory")

	// Verify destination now has the new content and old destination is gone
	require.FileExists(t, filepath.Join(dstDir, "file.txt"))
	require.NoFileExists(t, filepath.Join(dstDir, "old.txt"))
	require.NoDirExists(t, srcDir)
}

// TestProof_WindowsOSRenameFailsWhenTargetIsDirectory proves Claim 1 (target is directory):
// When moving a file over an existing directory path, standard os.Rename fails on Windows,
// while driver.Move (with the fallback) succeeds in replacing it.
func TestProof_WindowsOSRenameFailsWhenTargetIsDirectory(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "src_file.txt")
	dstDir := filepath.Join(tempDir, "dst_dir")

	require.NoError(t, os.WriteFile(srcFile, []byte("file data"), 0o644))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Proof: os.Rename from file to directory fails on Windows
	errStandardRename := os.Rename(srcFile, dstDir)
	require.Error(t, errStandardRename, "Standard os.Rename from file to existing directory must fail on Windows")
	t.Logf("[PROOF] Standard os.Rename from file to directory failed as expected: %v", errStandardRename)

	// Proof: driver rename fallback removes existing directory and succeeds
	errFallbackRename := rename(srcFile, dstDir)
	require.NoError(t, errFallbackRename, "Fallback rename must succeed replacing directory with file")
	t.Logf("[PROOF] Fallback rename from file to existing directory succeeded")

	fi, err := os.Stat(dstDir)
	require.NoError(t, err)
	require.False(t, fi.IsDir(), "Destination should now be a regular file")
}

// TestProof_WindowsOpenHandleBlocksRename proves Claim 1 (handles):
// On Windows, holding an open file handle blocks renaming/replacement due to sharing
// semantics (unlike POSIX where rename works on open file descriptors), requiring
// replace() to explicitly close the writer before calling Move.
func TestProof_WindowsOpenHandleBlocksRename(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "open_src.tmp")
	dstFile := filepath.Join(tempDir, "target.txt")

	f, err := os.OpenFile(srcFile, os.O_CREATE|os.O_RDWR, 0o666)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString("locked payload")
	require.NoError(t, err)

	// Proof Part A: os.Rename fails on Windows while handle is still open
	renameOpenErr := os.Rename(srcFile, dstFile)
	require.Error(t, renameOpenErr, "os.Rename must fail on Windows when the source file handle is open")
	t.Logf("[PROOF] os.Rename on open file handle failed as expected: %v", renameOpenErr)

	// Proof Part B: Closing the handle allows rename/replace to proceed
	require.NoError(t, f.Close())
	require.NoError(t, os.Rename(srcFile, dstFile))
	require.FileExists(t, dstFile)
	t.Logf("[PROOF] os.Rename succeeded after closing the open file handle")
}

// TestProof_WindowsDirectorySyncUnsupported proves Claim 3 (syncDir):
// On Windows, calling Sync() on a directory handle returns an error, proving why
// syncDir() must be a no-op on Windows.
func TestProof_WindowsDirectorySyncUnsupported(t *testing.T) {
	tempDir := t.TempDir()

	dirF, err := os.Open(tempDir)
	require.NoError(t, err)
	defer dirF.Close()

	// Proof: Syncing a directory on Windows fails
	syncErr := dirF.Sync()
	require.Error(t, syncErr, "dir.Sync() must fail on Windows")
	t.Logf("[PROOF] dir.Sync() on Windows returned expected error: %v", syncErr)

	// Proof: driver's syncDir() handles this cleanly as a no-op
	require.NoError(t, syncDir(tempDir))
}

// TestProof_StorageDriverMoveContractParity proves Claim 1 (driver contract):
// storagedriver.StorageDriver.Move(ctx, source, dest) replaces the destination
// if it already exists (both for file-to-file and directory structures).
func TestProof_StorageDriverMoveContractParity(t *testing.T) {
	d := newTestDriver(t)
	ctx := context.Background()

	// Case 1: File overwrite
	require.NoError(t, d.PutContent(ctx, "/file1.txt", []byte("initial content")))
	require.NoError(t, d.PutContent(ctx, "/file2.txt", []byte("updated content")))
	require.NoError(t, d.Move(ctx, "/file2.txt", "/file1.txt"))

	content, err := d.GetContent(ctx, "/file1.txt")
	require.NoError(t, err)
	require.Equal(t, []byte("updated content"), content)

	// Case 2: Directory overwrite
	require.NoError(t, d.PutContent(ctx, "/dirA/data.txt", []byte("dirA data")))
	require.NoError(t, d.PutContent(ctx, "/dirB/data.txt", []byte("dirB data")))
	require.NoError(t, d.Move(ctx, "/dirA", "/dirB"))

	dirBContent, err := d.GetContent(ctx, "/dirB/data.txt")
	require.NoError(t, err)
	require.Equal(t, []byte("dirA data"), dirBContent)

	_, err = d.GetContent(ctx, "/dirA/data.txt")
	require.Error(t, err)
	require.IsType(t, storagedriver.PathNotFoundError{}, err)
}

// TestProof_CASInvariantAndIdempotency proves Claim 2 (CAS Safety):
// Persistent assets stored under digest-keyed paths maintain determinism and idempotency.
// Intermediate uploads use unique UUID staging paths and overwrite destination identically.
func TestProof_CASInvariantAndIdempotency(t *testing.T) {
	d := newTestDriver(t)
	ctx := context.Background()

	destDigestPath := "/blobs/sha256/2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	blobContent := []byte("hello cas world")

	// Simulate multiple writes/moves to the same content-addressed destination
	for i := 0; i < 5; i++ {
		stagePath := fmt.Sprintf("/_uploads/%s", uuid.NewString())
		require.NoError(t, d.PutContent(ctx, stagePath, blobContent))
		require.NoError(t, d.Move(ctx, stagePath, destDigestPath))

		// Content at destination is always intact and identical
		got, err := d.GetContent(ctx, destDigestPath)
		require.NoError(t, err)
		require.Equal(t, blobContent, got)
	}

	// Also verify PutContent directly
	require.NoError(t, d.PutContent(ctx, destDigestPath, blobContent))
	got, err := d.GetContent(ctx, destDigestPath)
	require.NoError(t, err)
	require.Equal(t, blobContent, got)
}

// TestProof_HappyPathDirectRenameNoPenalty proves Claim 2.3:
// When moving to a non-existent destination path, os.Rename succeeds directly on the
// first try without needing or triggering the fallback removal path.
func TestProof_HappyPathDirectRenameNoPenalty(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "stage.tmp")
	dstFile := filepath.Join(tempDir, "final.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("happy path data"), 0o644))

	// First rename succeeds without dest existing
	err := rename(srcFile, dstFile)
	require.NoError(t, err)
	require.FileExists(t, dstFile)
	require.NoFileExists(t, srcFile)
	t.Logf("[PROOF] Direct rename succeeded on clean path without fallback")
}

// TestWindowsRenameReplacesExistingFile checks that a plain os.Rename already
// replaces an existing destination file on Windows (MoveFileEx with
// MOVEFILE_REPLACE_EXISTING), so the fallback in rename() is not involved.
func TestWindowsRenameReplacesExistingFile(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "src.txt")
	dstFile := filepath.Join(tempDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("old content"), 0o644))

	require.NoError(t, os.Rename(srcFile, dstFile))

	got, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	require.Equal(t, []byte("new content"), got)
	require.NoFileExists(t, srcFile)
}

// TestWindowsRenameLockedSourceKeepsDestination covers the review concern on
// rename(): when the source is held open by someone else, the rename fails,
// and the existing destination must still be there afterwards.
func TestWindowsRenameLockedSourceKeepsDestination(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "src.txt")
	dstFile := filepath.Join(tempDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("old content"), 0o644))

	// Go opens files without FILE_SHARE_DELETE, so this handle blocks
	// renaming the source.
	f, err := os.Open(srcFile)
	require.NoError(t, err)
	defer f.Close()

	err = rename(srcFile, dstFile)
	require.Error(t, err)

	got, err := os.ReadFile(dstFile)
	require.NoError(t, err, "destination must survive a failed rename")
	require.Equal(t, []byte("old content"), got)
}

// TestWindowsRenameLockedDestinationKeepsDestination checks the other side:
// when the destination is held open (e.g. by a concurrent reader), the rename
// fails and the destination keeps its old content.
func TestWindowsRenameLockedDestinationKeepsDestination(t *testing.T) {
	tempDir := t.TempDir()

	srcFile := filepath.Join(tempDir, "src.txt")
	dstFile := filepath.Join(tempDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("old content"), 0o644))

	f, err := os.Open(dstFile)
	require.NoError(t, err)
	defer f.Close()

	err = rename(srcFile, dstFile)
	require.Error(t, err)

	got, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	require.Equal(t, []byte("old content"), got)
	require.FileExists(t, srcFile)
}
