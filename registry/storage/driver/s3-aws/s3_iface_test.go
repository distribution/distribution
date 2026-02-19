package s3

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
)

type mockS3Client struct {
	objects map[string]*s3.Object
	mu      sync.RWMutex
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{
		objects: make(map[string]*s3.Object),
	}
}

func (m *mockS3Client) addObject(key string, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.objects[key] = &s3.Object{
		Key:          aws.String(key),
		Size:         aws.Int64(size),
		LastModified: &now,
	}
}

func (m *mockS3Client) GetObjectWithContext(context.Context, *s3.GetObjectInput, ...request.Option) (*s3.GetObjectOutput, error) {
	return nil, nil
}

func (m *mockS3Client) PutObjectWithContext(context.Context, *s3.PutObjectInput, ...request.Option) (*s3.PutObjectOutput, error) {
	return nil, nil
}

func (m *mockS3Client) AbortMultipartUploadWithContext(context.Context, *s3.AbortMultipartUploadInput, ...request.Option) (*s3.AbortMultipartUploadOutput, error) {
	return nil, nil
}

func (m *mockS3Client) CreateMultipartUploadWithContext(context.Context, *s3.CreateMultipartUploadInput, ...request.Option) (*s3.CreateMultipartUploadOutput, error) {
	return nil, nil
}

func (m *mockS3Client) CompleteMultipartUploadWithContext(context.Context, *s3.CompleteMultipartUploadInput, ...request.Option) (*s3.CompleteMultipartUploadOutput, error) {
	return nil, nil
}

func (m *mockS3Client) ListMultipartUploadsWithContext(context.Context, *s3.ListMultipartUploadsInput, ...request.Option) (*s3.ListMultipartUploadsOutput, error) {
	return nil, nil
}

func (m *mockS3Client) ListPartsWithContext(context.Context, *s3.ListPartsInput, ...request.Option) (*s3.ListPartsOutput, error) {
	return nil, nil
}

func (m *mockS3Client) HeadObjectWithContext(context.Context, *s3.HeadObjectInput, ...request.Option) (*s3.HeadObjectOutput, error) {
	return nil, nil
}

func (m *mockS3Client) ListObjectsV2WithContext(context.Context, *s3.ListObjectsV2Input, ...request.Option) (*s3.ListObjectsV2Output, error) {
	return nil, nil
}

func (m *mockS3Client) ListObjectsV2PagesWithContext(ctx context.Context, input *s3.ListObjectsV2Input, fn func(*s3.ListObjectsV2Output, bool) bool, opts ...request.Option) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := aws.StringValue(input.Prefix)
	delimiter := aws.StringValue(input.Delimiter)
	startAfter := aws.StringValue(input.StartAfter)
	maxKeys := aws.Int64Value(input.MaxKeys)
	if maxKeys == 0 {
		maxKeys = 1000
	}

	contents := make([]*s3.Object, 0, len(m.objects))
	prefixMap := make(map[string]bool)

	// Collect all matching keys
	allKeys := make([]string, 0, len(m.objects))
	for key := range m.objects {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if startAfter != "" && key <= startAfter {
			continue
		}
		allKeys = append(allKeys, key)
	}

	// Sort keys to match S3 behavior
	sort.Strings(allKeys)

	// Process keys based on delimiter
	for _, key := range allKeys {
		if delimiter != "" {
			// With delimiter, separate files from directories
			relativePath := strings.TrimPrefix(key, prefix)
			delimiterIndex := strings.Index(relativePath, delimiter)

			if delimiterIndex > 0 {
				// This is a subdirectory
				commonPrefix := prefix + relativePath[:delimiterIndex+1]
				prefixMap[commonPrefix] = true
				continue
			}
		}

		// This is a file at the current level
		contents = append(contents, m.objects[key])
	}

	// Convert prefix map to CommonPrefixes slice
	var commonPrefixes []*s3.CommonPrefix
	if delimiter != "" {
		prefixList := make([]string, 0, len(prefixMap))
		for p := range prefixMap {
			prefixList = append(prefixList, p)
		}
		sort.Strings(prefixList)
		for _, p := range prefixList {
			commonPrefixes = append(commonPrefixes, &s3.CommonPrefix{
				Prefix: aws.String(p),
			})
		}
	}

	// Simulate pagination if we have more results than maxKeys
	totalResults := int64(len(contents) + len(commonPrefixes))
	isLastPage := totalResults <= maxKeys

	// For simplicity in tests, we'll return everything in one page
	// A more sophisticated implementation would chunk results by maxKeys
	output := &s3.ListObjectsV2Output{
		Contents:       contents,
		CommonPrefixes: commonPrefixes,
		IsTruncated:    aws.Bool(!isLastPage),
	}

	// Call the callback with the output and whether this is the last page
	shouldContinue := fn(output, isLastPage)

	// In real S3, if callback returns false, we stop pagination
	if !shouldContinue {
		return nil
	}

	return nil
}

func (m *mockS3Client) CopyObjectWithContext(context.Context, *s3.CopyObjectInput, ...request.Option) (*s3.CopyObjectOutput, error) {
	return nil, nil
}

func (m *mockS3Client) UploadPartWithContext(context.Context, *s3.UploadPartInput, ...request.Option) (*s3.UploadPartOutput, error) {
	return nil, nil
}

func (m *mockS3Client) UploadPartCopyWithContext(context.Context, *s3.UploadPartCopyInput, ...request.Option) (*s3.UploadPartCopyOutput, error) {
	return nil, nil
}

func (m *mockS3Client) DeleteObjectsWithContext(aws.Context, *s3.DeleteObjectsInput, ...request.Option) (*s3.DeleteObjectsOutput, error) {
	return nil, nil
}

func (m *mockS3Client) GetObjectRequest(*s3.GetObjectInput) (*request.Request, *s3.GetObjectOutput) {
	return nil, nil
}

func (m *mockS3Client) HeadObjectRequest(*s3.HeadObjectInput) (*request.Request, *s3.HeadObjectOutput) {
	return nil, nil
}
