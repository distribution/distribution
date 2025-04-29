//go:build arm

package s3

import "math"

// maxChunkSize defines the maximum multipart upload chunk size allowed by S3.
// S3 API requires max upload chunk to be 5GB, but this overflows on 32-bit
// platforms.
const maxChunkSize = math.MaxInt32
