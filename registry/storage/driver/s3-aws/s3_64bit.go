//go:build !arm

package s3

// maxChunkSize defines the maximum multipart upload chunk size allowed by S3.
// S3 API requires max upload chunk to be 5GB.
const maxChunkSize = 5 * 1024 * 1024 * 1024
