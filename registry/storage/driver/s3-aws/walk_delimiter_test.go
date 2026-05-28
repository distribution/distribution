package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

func TestDirectoryDiff(t *testing.T) {
	tests := []struct {
		name     string
		prev     string
		current  string
		expected []string
	}{
		{
			name:     "one level deep",
			prev:     "/path/to/folder",
			current:  "/path/to/folder/folder/file",
			expected: []string{"/path/to/folder/folder"},
		},
		{
			name:     "different siblings",
			prev:     "/path/to/folder/folder1",
			current:  "/path/to/folder/folder2/file",
			expected: []string{"/path/to/folder/folder2"},
		},
		{
			name:     "different sibling files",
			prev:     "/path/to/folder/folder1/file",
			current:  "/path/to/folder/folder2/file",
			expected: []string{"/path/to/folder/folder2"},
		},
		{
			name:     "two levels deep",
			prev:     "/path/to/folder/folder1/file",
			current:  "/path/to/folder/folder2/folder1/file",
			expected: []string{"/path/to/folder/folder2", "/path/to/folder/folder2/folder1"},
		},
		{
			name:     "from root",
			prev:     "/",
			current:  "/path/to/folder/folder/file",
			expected: []string{"/path", "/path/to", "/path/to/folder", "/path/to/folder/folder"},
		},
		{
			name:     "empty prev",
			prev:     "",
			current:  "/path/to/file",
			expected: []string{},
		},
		{
			name:     "empty current",
			prev:     "/path",
			current:  "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := directoryDiff(tt.prev, tt.current)
			if len(result) != len(tt.expected) {
				t.Errorf("directoryDiff() = %v, expected %v", result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("directoryDiff()[%d] = %q, expected %q", i, result[i], tt.expected[i])
					return
				}
			}
		})
	}
}

func TestDriverS3Path(t *testing.T) {
	tests := []struct {
		name          string
		rootDirectory string
		path          string
		expected      string
	}{
		{
			name:          "empty root with slash path",
			rootDirectory: "",
			path:          "/file",
			expected:      "file",
		},
		{
			name:          "empty root with nested path",
			rootDirectory: "",
			path:          "/folder/file",
			expected:      "folder/file",
		},
		{
			name:          "root directory with slash",
			rootDirectory: "/root",
			path:          "/file",
			expected:      "root/file",
		},
		{
			name:          "root directory without trailing slash",
			rootDirectory: "root",
			path:          "/folder/file",
			expected:      "root/folder/file",
		},
		{
			name:          "root directory with trailing slash",
			rootDirectory: "root/",
			path:          "/folder/file",
			expected:      "root/folder/file",
		},
		{
			name:          "nested root directory",
			rootDirectory: "docker/registry/v2/repositories",
			path:          "/myrepo/_manifests",
			expected:      "docker/registry/v2/repositories/myrepo/_manifests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &driver{
				RootDirectory: tt.rootDirectory,
			}
			result := d.s3Path(tt.path)
			if result != tt.expected {
				t.Errorf("s3Path(%q) with root %q = %q, expected %q", tt.path, tt.rootDirectory, result, tt.expected)
			}
		})
	}
}

func TestDriverS3PathWithTrailingSlash(t *testing.T) {
	d := &driver{
		RootDirectory: "testroot",
	}

	path := "/folder1/subfolder1"
	s3Prefix := d.s3Path(path)

	if s3Prefix != "testroot/folder1/subfolder1" {
		t.Errorf("s3Path(%q) = %q, expected %q", path, s3Prefix, "testroot/folder1/subfolder1")
	}

	// Ensure trailing slash is not added by s3Path itself
	// (walkDirectory should add it)
	if s3Prefix[len(s3Prefix)-1] == '/' {
		t.Errorf("s3Path should not add trailing slash, got %q", s3Prefix)
	}
}

func TestBuildInitialStack(t *testing.T) {
	tests := []struct {
		name       string
		from       string
		startAfter string
		expected   []walkItem
	}{
		{
			name:       "no startAfter",
			from:       "/",
			startAfter: "",
			expected: []walkItem{
				{path: "/", startAfter: ""},
			},
		},
		{
			name:       "startAfter at root level",
			from:       "/",
			startAfter: "/library/nginx/_manifests",
			expected: []walkItem{
				{path: "/", startAfter: "/library/nginx/_manifests"},
				{path: "/library", startAfter: "/library/nginx/_manifests"},
			},
		},
		{
			name:       "startAfter deeply nested",
			from:       "/",
			startAfter: "/a/b/c/repo/_manifests",
			expected: []walkItem{
				{path: "/", startAfter: "/a/b/c/repo/_manifests"},
				{path: "/a", startAfter: "/a/b/c/repo/_manifests"},
				{path: "/a/b", startAfter: "/a/b/c/repo/_manifests"},
				{path: "/a/b/c", startAfter: "/a/b/c/repo/_manifests"},
			},
		},
		{
			name:       "from equals initialPath",
			from:       "/library",
			startAfter: "/library/nginx/_manifests",
			expected: []walkItem{
				{path: "/library", startAfter: "/library/nginx/_manifests"},
			},
		},
		{
			name:       "from is nested, startAfter deeper",
			from:       "/a/b",
			startAfter: "/a/b/c/d/repo/_manifests",
			expected: []walkItem{
				{path: "/a/b", startAfter: "/a/b/c/d/repo/_manifests"},
				{path: "/a/b/c", startAfter: "/a/b/c/d/repo/_manifests"},
				{path: "/a/b/c/d", startAfter: "/a/b/c/d/repo/_manifests"},
			},
		},
		{
			name:       "empty from with deeply nested startAfter",
			from:       "",
			startAfter: "/mycompany/another/nested/app",
			expected: []walkItem{
				{path: "", startAfter: "/mycompany/another/nested/app"},
				{path: "/mycompany", startAfter: "/mycompany/another/nested/app"},
				{path: "/mycompany/another", startAfter: "/mycompany/another/nested/app"},
				{path: "/mycompany/another/nested", startAfter: "/mycompany/another/nested/app"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildInitialStack(tt.from, tt.startAfter)

			if len(result) != len(tt.expected) {
				t.Errorf("buildInitialStack() returned %d items, expected %d", len(result), len(tt.expected))
				t.Errorf("result: %+v", result)
				t.Errorf("expected: %+v", tt.expected)
				return
			}

			for i := range result {
				if result[i].path != tt.expected[i].path || result[i].startAfter != tt.expected[i].startAfter {
					t.Errorf("buildInitialStack()[%d] = %+v, expected %+v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestDoWalkWithDelimiterMock(t *testing.T) {
	mockClient := newMockS3Client()

	// Create realistic Docker registry paths
	// Structure: docker/registry/v2/repositories/{repo}/_manifests, _layers, _uploads

	// Top-level repositories
	mockClient.addObject("docker/registry/v2/repositories/postgres/_manifests/revisions/sha256/aaa111/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/postgres/_manifests/tags/16/current/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/postgres/_layers/sha256/bbb222/link", 71)

	mockClient.addObject("docker/registry/v2/repositories/redis/_manifests/revisions/sha256/ccc333/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/redis/_manifests/tags/7/current/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/redis/_layers/sha256/ddd444/link", 71)

	mockClient.addObject("docker/registry/v2/repositories/mysql/_manifests/revisions/sha256/eee555/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/mysql/_layers/sha256/fff666/link", 71)

	// Nested repositories
	mockClient.addObject("docker/registry/v2/repositories/library/nginx/_manifests/revisions/sha256/abc123/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/library/nginx/_manifests/tags/latest/current/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/library/nginx/_layers/sha256/def456/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/library/nginx/_uploads/uuid-123/startedat", 30)

	mockClient.addObject("docker/registry/v2/repositories/library/ubuntu/_manifests/revisions/sha256/xyz789/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/library/ubuntu/_layers/sha256/aaa999/link", 71)

	mockClient.addObject("docker/registry/v2/repositories/mycompany/myapp/_manifests/revisions/sha256/bbb222/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/mycompany/myapp/_manifests/tags/v1.0/current/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/mycompany/myapp/_layers/sha256/ccc333/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/mycompany/myapp/_layers/sha256/ddd444/link", 71)

	mockClient.addObject("docker/registry/v2/repositories/mycompany/another/nested/app/_manifests/revisions/sha256/eee555/link", 71)

	// Add repos that come alphabetically BEFORE library to test proper handling
	mockClient.addObject("docker/registry/v2/repositories/alpine/_manifests/revisions/sha256/aaa111/link", 71)
	mockClient.addObject("docker/registry/v2/repositories/busybox/_manifests/revisions/sha256/bbb222/link", 71)

	d := &driver{
		S3:               mockClient,
		Bucket:           "test-bucket",
		RootDirectory:    "docker/registry/v2/repositories",
		UseDelimiterWalk: true,
	}

	tests := []struct {
		name           string
		from           string
		startAfter     string
		stopAt         string
		expectedWalked []string
		expectedRepos  []string
	}{
		{
			name:       "catalog: walk all repos and skip underscore dirs",
			from:       "/",
			startAfter: "",
			expectedRepos: []string{
				"alpine",
				"busybox",
				"library/nginx",
				"library/ubuntu",
				"mycompany/another/nested/app",
				"mycompany/myapp",
				"mysql",
				"postgres",
				"redis",
			},
		},
		{
			name:       "walk from specific repo",
			from:       "/library/nginx",
			startAfter: "",
			expectedRepos: []string{
				"library/nginx",
			},
			expectedWalked: []string{
				"/library/nginx/_layers",
				"/library/nginx/_manifests",
				"/library/nginx/_uploads",
			},
		},
		{
			name:       "start after library/nginx",
			from:       "/",
			startAfter: "/library/nginx/_uploads/uuid-123/startedat",
			expectedRepos: []string{
				"library/ubuntu",
				"mycompany/another/nested/app",
				"mycompany/myapp",
				"mysql",
				"postgres",
				"redis",
			},
		},
		{
			name:       "start after deeply nested repo",
			from:       "",
			startAfter: "/mycompany/another/nested/app",
			expectedRepos: []string{
				"mycompany/myapp",
				"mysql",
				"postgres",
				"redis",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var walked []string
			var repos []string
			var mu sync.Mutex
			var objectCount int64
			// root := d.RootDirectory

			walkFn := func(fileInfo storagedriver.FileInfo) error {
				mu.Lock()
				walked = append(walked, fileInfo.Path())
				mu.Unlock()

				// Always use HandleRepository to mimic catalog behavior
				// Root is empty "" because paths from walk are registry-relative (start with /)
				return storage.HandleRepository(fileInfo, "", "", func(repoPath string) error {
					mu.Lock()
					repos = append(repos, repoPath)
					mu.Unlock()
					return nil
				})
			}

			err := d.doWalkWithDelimiter(context.Background(), &objectCount, tt.from, tt.startAfter, walkFn)
			if err != nil {
				t.Fatalf("doWalkWithDelimiter() error = %v", err)
			}

			// Check repos
			if len(repos) != len(tt.expectedRepos) {
				t.Errorf("found %d repos, expected %d", len(repos), len(tt.expectedRepos))
				t.Errorf("repos: %v", repos)
				t.Errorf("expected: %v", tt.expectedRepos)
				return
			}

			for i := range repos {
				if repos[i] != tt.expectedRepos[i] {
					t.Errorf("repos[%d] = %q, expected %q", i, repos[i], tt.expectedRepos[i])
				}
			}

			// Check walked paths if expectedWalked is set
			if tt.expectedWalked != nil {
				if len(walked) != len(tt.expectedWalked) {
					t.Errorf("walked %d items, expected %d", len(walked), len(tt.expectedWalked))
					t.Errorf("walked: %v", walked)
					t.Errorf("expected: %v", tt.expectedWalked)
					return
				}

				for i := range walked {
					if walked[i] != tt.expectedWalked[i] {
						t.Errorf("walked[%d] = %q, expected %q", i, walked[i], tt.expectedWalked[i])
					}
				}
			}
		})
	}
}

func TestDoWalkWithDelimiterConcurrency(t *testing.T) {
	mockClient := newMockS3Client()

	// Create many repos to test concurrent recursion
	for i := 0; i < 10; i++ {
		for j := 0; j < 5; j++ {
			mockClient.addObject(fmt.Sprintf("docker/registry/v2/repositories/repo%d/_manifests/revisions/sha256/hash%d/link", i, j), 71)
		}
	}

	d := &driver{
		S3:               mockClient,
		Bucket:           "test-bucket",
		RootDirectory:    "docker/registry/v2/repositories",
		UseDelimiterWalk: true,
	}

	var walked []string
	var mu sync.Mutex
	var objectCount int64

	walkFn := func(fileInfo storagedriver.FileInfo) error {
		mu.Lock()
		walked = append(walked, fileInfo.Path())
		mu.Unlock()
		return nil
	}

	err := d.doWalkWithDelimiter(context.Background(), &objectCount, "/", "", walkFn)
	if err != nil {
		t.Fatalf("doWalkWithDelimiter() error = %v", err)
	}

	// Should have many directories + 50 files
	// Each repo has: repo%d, _manifests, revisions, sha256, hash%d (5 times), link (5 times)
	// 10 repos * (1 + 1 + 1 + 1 + 5 + 5) = 10 * 14 = 140 items
	expectedCount := 140
	if len(walked) != expectedCount {
		t.Errorf("Expected %d items, got %d", expectedCount, len(walked))
	}
}

func TestShouldSkipDirectory(t *testing.T) {
	tests := []struct {
		name        string
		dirPath     string
		startAfter  string
		shouldSkip  bool
		description string
	}{
		{
			name:        "no startAfter",
			dirPath:     "/mycompany/myapp",
			startAfter:  "",
			shouldSkip:  false,
			description: "When startAfter is empty, never skip",
		},
		{
			name:        "dirPath equals startAfter",
			dirPath:     "/mycompany/another/nested/app",
			startAfter:  "/mycompany/another/nested/app",
			shouldSkip:  true,
			description: "When dirPath equals startAfter, skip it",
		},
		{
			name:        "dirPath before startAfter",
			dirPath:     "/alpine",
			startAfter:  "/library/nginx/_manifests",
			shouldSkip:  true,
			description: "Alpine comes before library, should skip",
		},
		{
			name:        "dirPath after startAfter",
			dirPath:     "/mycompany/myapp",
			startAfter:  "/mycompany/another/nested/app",
			shouldSkip:  false,
			description: "myapp comes after another/nested/app, should not skip",
		},
		{
			name:        "sibling directory after startAfter",
			dirPath:     "/library/ubuntu",
			startAfter:  "/library/nginx/_uploads/uuid-123/startedat",
			shouldSkip:  false,
			description: "ubuntu comes after nginx alphabetically, should not skip",
		},
		{
			name:        "directory at root level",
			dirPath:     "/mysql",
			startAfter:  "/mycompany/another/nested/app",
			shouldSkip:  false,
			description: "mysql (root level) comes after mycompany at first character, should not skip",
		},
		{
			name:        "nested dir within startAfter path",
			dirPath:     "/mycompany/another",
			startAfter:  "/mycompany/another/nested/app",
			shouldSkip:  true,
			description: "Parent directory of startAfter should be skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipDirectory(tt.dirPath, tt.startAfter)
			if result != tt.shouldSkip {
				t.Errorf("%s: shouldSkipDirectory(%q, %q) = %v, want %v",
					tt.description, tt.dirPath, tt.startAfter, result, tt.shouldSkip)
			}
		})
	}
}
