package storage

import (
	"path"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
)

var pm = defaultPathMapper

func testUploadFS(t *testing.T, numUploads int, repoName string, startedAt time.Time) driver.StorageDriver {
	d := inmemory.New()
	for i := 0; i < numUploads; i++ {
		addUploads(t, d, uuid.New(), repoName, startedAt)
	}
	return d
}

func addUploads(t *testing.T, d driver.StorageDriver, uploadID, repo string, startedAt time.Time) {
	dataPath, err := pm.path(uploadDataPathSpec{name: repo, uuid: uploadID})
	if err != nil {
		t.Fatalf("Unable to resolve path")
	}
	if err := d.PutContent(dataPath, []byte("")); err != nil {
		t.Fatalf("Unable to write data file")
	}

	startedAtPath, err := pm.path(uploadStartedAtPathSpec{name: repo, uuid: uploadID})
	if err != nil {
		t.Fatalf("Unable to resolve path")
	}

	if d.PutContent(startedAtPath, []byte(startedAt.Format(time.RFC3339))); err != nil {
		t.Fatalf("Unable to write startedAt file")
	}

}

func TestPurgeGather(t *testing.T) {
	uploadCount := 5
	fs := testUploadFS(t, uploadCount, "test-repo", time.Now())
	uploadData, errs := getOutstandingUploads(fs)
	if len(errs) != 0 {
		t.Errorf("Unexepected errors: %q", errs)
	}
	if len(uploadData) != uploadCount {
		t.Errorf("Unexpected upload file count: %d != %d", uploadCount, len(uploadData))
	}
}

func TestPurgeNone(t *testing.T) {
	fs := testUploadFS(t, 10, "test-repo", time.Now())
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	deleted, errs := PurgeUploads(fs, oneHourAgo, true)
	if len(errs) != 0 {
		t.Error("Unexpected errors", errs)
	}
	if len(deleted) != 0 {
		t.Errorf("Unexpectedly deleted files for time: %s", oneHourAgo)
	}
}

func TestPurgeAll(t *testing.T) {
	uploadCount := 10
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	fs := testUploadFS(t, uploadCount, "test-repo", oneHourAgo)

	// Ensure > 1 repos are purged
	addUploads(t, fs, uuid.New(), "test-repo2", oneHourAgo)
	uploadCount++

	deleted, errs := PurgeUploads(fs, time.Now(), true)
	if len(errs) != 0 {
		t.Error("Unexpected errors:", errs)
	}
	fileCount := uploadCount
	if len(deleted) != fileCount {
		t.Errorf("Unexpectedly deleted file count %d != %d",
			len(deleted), fileCount)
	}
}

func TestPurgeSome(t *testing.T) {
	oldUploadCount := 5
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	fs := testUploadFS(t, oldUploadCount, "library/test-repo", oneHourAgo)

	newUploadCount := 4

	for i := 0; i < newUploadCount; i++ {
		addUploads(t, fs, uuid.New(), "test-repo", time.Now().Add(1*time.Hour))
	}

	deleted, errs := PurgeUploads(fs, time.Now(), true)
	if len(errs) != 0 {
		t.Error("Unexpected errors:", errs)
	}
	if len(deleted) != oldUploadCount {
		t.Errorf("Unexpectedly deleted file count %d != %d",
			len(deleted), oldUploadCount)
	}
}

func TestPurgeOnlyUploads(t *testing.T) {
	oldUploadCount := 5
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	fs := testUploadFS(t, oldUploadCount, "test-repo", oneHourAgo)

	// Create a directory tree outside _uploads and ensure
	// these files aren't deleted.
	dataPath, err := pm.path(uploadDataPathSpec{name: "test-repo", uuid: uuid.New()})
	if err != nil {
		t.Fatalf(err.Error())
	}
	nonUploadPath := strings.Replace(dataPath, "_upload", "_important", -1)
	if strings.Index(nonUploadPath, "_upload") != -1 {
		t.Fatalf("Non-upload path not created correctly")
	}

	nonUploadFile := path.Join(nonUploadPath, "file")
	if err = fs.PutContent(nonUploadFile, []byte("")); err != nil {
		t.Fatalf("Unable to write data file")
	}

	deleted, errs := PurgeUploads(fs, time.Now(), true)
	if len(errs) != 0 {
		t.Error("Unexpected errors", errs)
	}
	for _, file := range deleted {
		if strings.Index(file, "_upload") == -1 {
			t.Errorf("Non-upload file deleted")
		}
	}
}

func TestPurgeMissingStartedAt(t *testing.T) {
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	fs := testUploadFS(t, 1, "test-repo", oneHourAgo)
	err := Walk(fs, "/", func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()
		_, file := path.Split(filePath)

		if file == "startedat" {
			if err := fs.Delete(filePath); err != nil {
				t.Fatalf("Unable to delete startedat file: %s", filePath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error during Walk: %s ", err.Error())
	}
	deleted, errs := PurgeUploads(fs, time.Now(), true)
	if len(errs) > 0 {
		t.Errorf("Unexpected errors")
	}
	if len(deleted) > 0 {
		t.Errorf("Files unexpectedly deleted: %s", deleted)
	}
}
