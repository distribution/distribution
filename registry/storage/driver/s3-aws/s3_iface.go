package s3

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3Client is a client-side interface for [github.com/aws/aws-sdk-go/service/s3.S3]
type S3Client interface {
	GetObjectWithContext(context.Context, *s3.GetObjectInput, ...request.Option) (*s3.GetObjectOutput, error)
	PutObjectWithContext(context.Context, *s3.PutObjectInput, ...request.Option) (*s3.PutObjectOutput, error)
	AbortMultipartUploadWithContext(context.Context, *s3.AbortMultipartUploadInput, ...request.Option) (*s3.AbortMultipartUploadOutput, error)
	CreateMultipartUploadWithContext(context.Context, *s3.CreateMultipartUploadInput, ...request.Option) (*s3.CreateMultipartUploadOutput, error)
	CompleteMultipartUploadWithContext(context.Context, *s3.CompleteMultipartUploadInput, ...request.Option) (*s3.CompleteMultipartUploadOutput, error)
	ListMultipartUploadsWithContext(context.Context, *s3.ListMultipartUploadsInput, ...request.Option) (*s3.ListMultipartUploadsOutput, error)
	ListPartsWithContext(context.Context, *s3.ListPartsInput, ...request.Option) (*s3.ListPartsOutput, error)
	HeadObjectWithContext(context.Context, *s3.HeadObjectInput, ...request.Option) (*s3.HeadObjectOutput, error)
	ListObjectsV2WithContext(context.Context, *s3.ListObjectsV2Input, ...request.Option) (*s3.ListObjectsV2Output, error)
	ListObjectsV2PagesWithContext(context.Context, *s3.ListObjectsV2Input, func(*s3.ListObjectsV2Output, bool) bool, ...request.Option) error
	CopyObjectWithContext(context.Context, *s3.CopyObjectInput, ...request.Option) (*s3.CopyObjectOutput, error)
	UploadPartWithContext(context.Context, *s3.UploadPartInput, ...request.Option) (*s3.UploadPartOutput, error)
	UploadPartCopyWithContext(context.Context, *s3.UploadPartCopyInput, ...request.Option) (*s3.UploadPartCopyOutput, error)
	DeleteObjectsWithContext(aws.Context, *s3.DeleteObjectsInput, ...request.Option) (*s3.DeleteObjectsOutput, error)
	GetObjectRequest(*s3.GetObjectInput) (*request.Request, *s3.GetObjectOutput)
	HeadObjectRequest(*s3.HeadObjectInput) (*request.Request, *s3.HeadObjectOutput)
}
