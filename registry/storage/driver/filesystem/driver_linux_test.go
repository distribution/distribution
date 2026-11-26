// Licensed under the Apache License, Version 2.0.
// See LICENSE for the full license text.

package filesystem

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPutContentCommitFailureLeaksTempFile(t *testing.T) {
	// Isolate the process-wide limit from the test runner and other tests.
	if os.Getenv("GO_TEST_PUTCONTENT_COMMIT_FAILURE") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestPutContentCommitFailureLeaksTempFile$", "-test.v")
		cmd.Env = append(os.Environ(), "GO_TEST_PUTCONTENT_COMMIT_FAILURE=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}

	var original syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_FSIZE, &original))

	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "overwrite"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			d := New(DriverParameters{RootDirectory: root, MaxThreads: minThreads})
			ctx := context.Background()
			const key = "/uploads/id/startedat"
			if existing {
				require.NoError(t, d.PutContent(ctx, key, []byte("original")))
			}

			// The payload fits in the write buffer, but exceeds the file size
			// limit, so the write fails when Commit flushes the buffer.
			var err error
			func() {
				limit := original
				limit.Cur = 64
				require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit))
				defer func() {
					require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &original))
				}()
				err = d.PutContent(ctx, key, bytes.Repeat([]byte("x"), 128))
			}()
			require.Error(t, err)

			entries, err := os.ReadDir(filepath.Join(root, "uploads", "id"))
			require.NoError(t, err)
			if existing {
				got, err := d.GetContent(ctx, key)
				require.NoError(t, err)
				require.Equal(t, "original", string(got))
				require.Len(t, entries, 1, "failed commits must not leave temporary files")
				require.Equal(t, "startedat", entries[0].Name())
			} else {
				require.Empty(t, entries, "failed commits must not leave temporary files")
			}
		})
	}
}
