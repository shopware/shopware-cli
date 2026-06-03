package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Connection holds the connection details for an S3 compatible storage.
type S3Connection struct {
	// Endpoint is the S3 endpoint URL. Leave empty for AWS S3.
	Endpoint string
	// Region is the S3 region. Defaults to "us-east-1" when empty.
	Region string
	// AccessKey is the access key id.
	AccessKey string
	// SecretKey is the secret access key.
	SecretKey string
	// UsePathStyle forces path-style addressing (bucket in the path instead of
	// the host). Required by most S3-compatible providers such as MinIO.
	UsePathStyle bool
}

// S3Client wraps the AWS S3 client with the few operations the migration needs.
type S3Client struct {
	client *s3.Client
}

// NewS3Client builds an S3 client from the given connection details. It uses
// static credentials only and never falls back to ambient AWS environment
// variables or profiles, so the behaviour is predictable.
func NewS3Client(conn S3Connection) (*S3Client, error) {
	if conn.AccessKey == "" || conn.SecretKey == "" {
		return nil, errors.New("access key and secret key are required")
	}

	region := conn.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(conn.AccessKey, conn.SecretKey, ""),
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if conn.Endpoint != "" {
			o.BaseEndpoint = aws.String(conn.Endpoint)
		}
		o.UsePathStyle = conn.UsePathStyle
	})

	return &S3Client{client: client}, nil
}

// ValidateBucket checks that the bucket is reachable and that the credentials
// allow writing, reading and deleting objects. It performs a small write,
// read-back and delete of a probe object.
func (c *S3Client) ValidateBucket(ctx context.Context, bucket string) error {
	if _, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return fmt.Errorf("bucket %q is not reachable: %w", bucket, friendlyErr(err))
	}

	probeKey := fmt.Sprintf(".shopware-cli-probe-%d", time.Now().UnixNano())
	probeBody := []byte("shopware-cli storage migration probe")

	if _, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(probeKey),
		Body:   bytes.NewReader(probeBody),
	}); err != nil {
		return fmt.Errorf("cannot write to bucket %q: %w", bucket, friendlyErr(err))
	}

	// Always try to clean up the probe object, even when the read-back fails.
	defer func() {
		_, _ = c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(probeKey),
		})
	}()

	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(probeKey),
	})
	if err != nil {
		return fmt.Errorf("cannot read from bucket %q: %w", bucket, friendlyErr(err))
	}
	_ = out.Body.Close()

	return nil
}

// Stat returns the size of an object and whether it exists.
func (c *S3Client) Stat(ctx context.Context, bucket, key string) (int64, bool, error) {
	out, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return 0, false, nil
		}
		// Some providers return a generic 404 without the typed error.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey") {
			return 0, false, nil
		}
		return 0, false, friendlyErr(err)
	}

	if out.ContentLength == nil {
		return 0, true, nil
	}
	return *out.ContentLength, true, nil
}

// UploadOptions controls a single upload.
type UploadOptions struct {
	// PublicACL sets a public-read canned ACL on the uploaded object. Many
	// S3-compatible providers reject ACLs (e.g. Cloudflare R2), so this is
	// opt-in and public access is otherwise expected to come from a bucket
	// policy.
	PublicACL bool
}

// Upload streams a file to the bucket under the given key, setting a content
// type derived from the key extension. When r is a seekable reader (such as an
// *os.File) the body is streamed without buffering it fully in memory.
func (c *S3Client) Upload(ctx context.Context, bucket, key string, r io.Reader, opts UploadOptions) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType(key)),
	}

	if opts.PublicACL {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := c.client.PutObject(ctx, input); err != nil {
		return friendlyErr(err)
	}

	return nil
}

func contentType(key string) string {
	if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// friendlyErr unwraps smithy API errors into a more readable message.
func friendlyErr(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %s", apiErr.ErrorCode(), apiErr.ErrorMessage())
	}
	return err
}
