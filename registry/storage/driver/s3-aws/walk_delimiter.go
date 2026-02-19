package s3

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

type walkItem struct {
	path       string
	startAfter string
}

// getInitialPath optimizes the walk start position by finding the deepest common parent
// directory before a repository-specific directory (one that starts with underscore).
//
// For Docker Registry paths like /v2/repositories/org/repo/_manifests, this returns
// /v2/repositories/org (the parent of the repository), allowing the walk to skip
// listing from the root and start closer to the target.
//
// Returns empty string if:
//   - startAfter is empty
//   - No underscore-prefixed directory is found (not a registry path)
//   - The repository would be at the root level
func getInitialPath(startAfter string) string {
	if startAfter == "" {
		return ""
	}

	pathComponents := strings.Split(startAfter, "/")

	// Find the rightmost underscore-prefixed directory (e.g., _manifests, _layers)
	underscoreDirIndex := -1
	for i := len(pathComponents) - 1; i >= 0; i-- {
		if strings.HasPrefix(pathComponents[i], "_") {
			underscoreDirIndex = i
			break
		}
	}

	if underscoreDirIndex == -1 {
		return "" // Not a registry-structured path
	}

	// The repository name is immediately before the underscore directory
	// Return the parent of the repository (one level up)
	repoIndex := underscoreDirIndex - 1
	if repoIndex <= 0 {
		return "" // Repository is at root level
	}

	parentIndex := repoIndex
	return strings.Join(pathComponents[0:parentIndex], "/")
}

// buildInitialStack creates the initial directory stack for walking.
// The stack contains all directories that need to be explored, with startAfter
// propagated to help skip already-processed paths.
//
// The function optimizes the starting point using getInitialPath to avoid
// unnecessary directory traversal when possible.
func buildInitialStack(from, startAfter string) []walkItem {
	if startAfter == "" {
		return []walkItem{{path: from, startAfter: ""}}
	}

	initialPath := getInitialPath(startAfter)

	// Case 1: initialPath matches from exactly (both non-empty)
	if initialPath != "" && initialPath == from {
		return []walkItem{{path: from, startAfter: startAfter}}
	}

	// Case 2: No underscore dirs found and from is empty
	// Build intermediate paths from startAfter to ensure proper traversal
	if initialPath == "" && from == "" {
		return buildIntermediatePaths(startAfter)
	}

	// Case 3: No underscore dirs found but from is set
	if initialPath == "" {
		return []walkItem{{path: from, startAfter: startAfter}}
	}

	// Case 4: Build path hierarchy from 'from' down to 'initialPath'
	return buildPathHierarchy(from, initialPath, startAfter)
}

// buildIntermediatePaths creates a stack of all parent directories
// up to (but not including) the last component of startAfter.
// Example: "/a/b/c/d" -> ["", "/a", "/a/b", "/a/b/c"]
func buildIntermediatePaths(startAfter string) []walkItem {
	parts := strings.Split(strings.Trim(startAfter, "/"), "/")
	paths := make([]string, 0, len(parts))
	paths = append(paths, "")

	currentPath := ""
	for i := 0; i < len(parts)-1; i++ {
		currentPath = currentPath + "/" + parts[i]
		paths = append(paths, currentPath)
	}

	stack := make([]walkItem, 0, len(paths))
	for _, p := range paths {
		stack = append(stack, walkItem{path: p, startAfter: startAfter})
	}
	return stack
}

// buildPathHierarchy creates a stack containing all paths from 'from' down to 'to'.
// Example: from="/", to="/a/b" -> ["/", "/a", "/a/b"]
func buildPathHierarchy(from, to, startAfter string) []walkItem {
	var paths []string
	paths = append(paths, from)

	// Only build intermediate paths if 'to' is under 'from'
	if from == "" || strings.HasPrefix(to, from) {
		relativePath := strings.TrimPrefix(to, from)
		if from == "/" || from == "" {
			relativePath = strings.TrimPrefix(to, "")
		}
		relativePath = strings.Trim(relativePath, "/")

		if relativePath != "" {
			parts := strings.Split(relativePath, "/")
			currentPath := from
			if currentPath == "/" || currentPath == "" {
				currentPath = ""
			}

			for _, part := range parts {
				currentPath = currentPath + "/" + part
				if currentPath != to {
					paths = append(paths, currentPath)
				}
			}
		}
	}

	// Add the final path if different from start
	if to != from {
		paths = append(paths, to)
	}

	stack := make([]walkItem, 0, len(paths))
	for _, p := range paths {
		stack = append(stack, walkItem{path: p, startAfter: startAfter})
	}
	return stack
}

// shouldSkipDirectory determines if a directory should be skipped based on the startAfter hint.
// It returns true if the directory should be skipped (already processed or needs to be filtered).
func shouldSkipDirectory(dirPath, startAfter string) bool {
	if startAfter == "" {
		return false
	}

	// Skip if dirPath <= startAfter lexicographically
	return dirPath <= startAfter
}

// doWalkWithDelimiter implements directory traversal using S3's delimiter feature.
// This performs hierarchical walking by listing one directory level at a time,
// which is more efficient than the flat listing approach when StartAfter hints are used.
func (d *driver) doWalkWithDelimiter(parentCtx context.Context, objectCount *int64, from, startAfter string, f storagedriver.WalkFn) error {
	prefix := d.getPathPrefix()
	ctx, done := dcontext.WithTrace(parentCtx)
	defer done("s3aws.doWalkWithDelimiter(%s)", from)

	skipDirs := make(map[string]bool)
	stack := buildInitialStack(from, startAfter)

	for len(stack) > 0 {
		current := popStack(&stack)

		dirs, files, err := d.listDirectory(ctx, current, prefix, skipDirs)
		if err != nil {
			return err
		}

		// Process directories and collect child directories for further traversal
		childDirs, err := d.processDirectories(dirs, current, skipDirs, objectCount, f)
		if err != nil {
			if err == storagedriver.ErrFilledBuffer {
				return nil
			}
			return err
		}

		// Push child directories onto stack in reverse order for depth-first traversal
		d.pushChildDirectories(&stack, childDirs, current, skipDirs)

		// Process files at this level
		if err := d.processFiles(files, current, objectCount, f); err != nil {
			if err == storagedriver.ErrFilledBuffer {
				return nil
			}
			return err
		}
	}

	return nil
}

// getPathPrefix returns the prefix to add to registry paths based on root directory config
func (d *driver) getPathPrefix() string {
	if d.s3Path("") == "" {
		return "/"
	}
	return ""
}

// popStack removes and returns the top item from the stack
func popStack(stack *[]walkItem) walkItem {
	current := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	return current
}

// listDirectory lists all subdirectories and files at the current directory level
func (d *driver) listDirectory(ctx context.Context, current walkItem, prefix string, skipDirs map[string]bool) ([]string, []storagedriver.FileInfoInternal, error) {
	s3Prefix := d.s3Path(current.path)
	if !strings.HasSuffix(s3Prefix, "/") {
		s3Prefix = s3Prefix + "/"
	}

	listInput := &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.Bucket),
		Prefix:    aws.String(s3Prefix),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int64(listMax),
	}

	if current.startAfter != "" && strings.HasPrefix(current.startAfter, current.path) {
		listInput.StartAfter = aws.String(d.s3Path(current.startAfter))
	}

	var dirs []string
	var files []storagedriver.FileInfoInternal

	err := d.S3.ListObjectsV2PagesWithContext(ctx, listInput, func(output *s3.ListObjectsV2Output, lastPage bool) bool {
		// Collect directories (common prefixes)
		for _, commonPrefix := range output.CommonPrefixes {
			dirPath := strings.Replace(strings.TrimSuffix(*commonPrefix.Prefix, "/"), d.s3Path(""), prefix, 1)
			if !skipDirs[dirPath] {
				dirs = append(dirs, dirPath)
			}
		}

		// Collect files
		for _, obj := range output.Contents {
			filePath := strings.Replace(*obj.Key, d.s3Path(""), prefix, 1)
			if !strings.HasSuffix(filePath, "/") {
				files = append(files, storagedriver.FileInfoInternal{
					FileInfoFields: storagedriver.FileInfoFields{
						IsDir:   false,
						Size:    *obj.Size,
						ModTime: *obj.LastModified,
						Path:    filePath,
					},
				})
			}
		}

		return true
	})

	if err != nil {
		return nil, nil, err
	}

	// Sort for deterministic ordering
	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path() < files[j].Path()
	})

	return dirs, files, nil
}

// processDirectories calls the walk function on each directory and returns
// the list of directories that should be traversed further
func (d *driver) processDirectories(dirs []string, current walkItem, skipDirs map[string]bool, objectCount *int64, f storagedriver.WalkFn) ([]string, error) {
	childDirs := make([]string, 0, len(dirs))

	for _, dirPath := range dirs {
		// Skip directories based on startAfter hint
		if shouldSkipDirectory(dirPath, current.startAfter) {
			skipDirs[dirPath] = true
			continue
		}

		walkInfo := storagedriver.FileInfoInternal{
			FileInfoFields: storagedriver.FileInfoFields{
				IsDir: true,
				Path:  dirPath,
			},
		}

		*objectCount++
		err := f(walkInfo)

		if err != nil {
			if err == storagedriver.ErrSkipDir {
				skipDirs[dirPath] = true
				continue
			}
			if err == storagedriver.ErrFilledBuffer {
				return nil, storagedriver.ErrFilledBuffer
			}
			return nil, err
		}

		childDirs = append(childDirs, dirPath)
	}

	return childDirs, nil
}

// pushChildDirectories adds child directories to the stack for further traversal.
// Directories are pushed in reverse order so they pop in forward (alphabetical) order.
func (d *driver) pushChildDirectories(stack *[]walkItem, childDirs []string, current walkItem, skipDirs map[string]bool) {
	for i := len(childDirs) - 1; i >= 0; i-- {
		dirPath := childDirs[i]
		if skipDirs[dirPath] {
			continue
		}

		// Only pass startAfter to child if startAfter is within that child's path
		childStartAfter := ""
		if current.startAfter != "" && strings.HasPrefix(current.startAfter, dirPath+"/") {
			childStartAfter = current.startAfter
		}

		*stack = append(*stack, walkItem{path: dirPath, startAfter: childStartAfter})
	}
}

// processFiles calls the walk function on each file at the current level
func (d *driver) processFiles(files []storagedriver.FileInfoInternal, current walkItem, objectCount *int64, f storagedriver.WalkFn) error {
	for _, fileInfo := range files {
		// Skip files that are <= startAfter
		if current.startAfter != "" && fileInfo.Path() <= current.startAfter {
			continue
		}

		*objectCount++
		err := f(fileInfo)

		if err != nil {
			return err
		}
	}

	return nil
}
