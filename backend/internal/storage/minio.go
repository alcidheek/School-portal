package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOStorage manages objects inside a private MinIO bucket.
type MinIOStorage struct {
	client               *minio.Client
	bucket               string
	closeIdleConnections func()
}

// Close releases idle connections when the application shuts down.
func (storage *MinIOStorage) Close() {
	if storage.closeIdleConnections != nil {
		storage.closeIdleConnections()
	}
}

// NewMinIOStorage establishes the MinIO client and verifies
// that the configured bucket already exists.
func NewMinIOStorage(
	ctx context.Context,
	endpoint string,
	accessKey string,
	secretKey string,
	bucket string,
	useSSL bool,
) (*MinIOStorage, error) {
	if strings.TrimSpace(endpoint) == "" ||
		strings.TrimSpace(accessKey) == "" ||
		strings.TrimSpace(secretKey) == "" ||
		strings.TrimSpace(bucket) == "" {
		return nil, errors.New(
			"MinIO endpoint, credentials and bucket are required",
		)
	}

	transport, err := minio.DefaultTransport(useSSL)
	if err != nil {
		return nil, errors.New("initialize MinIO transport: unable to configure TLS")
	}
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second

	client, err := minio.New(
		endpoint,
		&minio.Options{
			Creds: credentials.NewStaticV4(
				accessKey,
				secretKey,
				"",
			),
			Secure:    useSSL,
			Transport: transport,
		},
	)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errors.New("initialize MinIO client: invalid endpoint or client configuration")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exists, err := client.BucketExists(checkCtx, bucket)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, safeStorageError("check MinIO bucket", err)
	}

	if !exists {
		transport.CloseIdleConnections()
		return nil, errors.New("configured MinIO bucket does not exist")
	}

	return &MinIOStorage{
		client:               client,
		bucket:               bucket,
		closeIdleConnections: transport.CloseIdleConnections,
	}, nil
}

// Do not propagate server messages, URLs, or credential-bearing errors to logs.
func safeStorageError(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", operation, context.Canceled)
	}
	switch minio.ToErrorResponse(err).Code {
	case "InvalidAccessKeyId", "SignatureDoesNotMatch", "AccessDenied", "InvalidToken":
		return fmt.Errorf("%s: authentication failed or bucket access denied; check application credentials and permissions", operation)
	case "NoSuchBucket":
		return fmt.Errorf("%s: configured MinIO bucket does not exist", operation)
	case "NoSuchKey":
		return fmt.Errorf("%s: object does not exist", operation)
	default:
		return fmt.Errorf("%s: request failed; check endpoint, connectivity, TLS and bucket configuration", operation)
	}
}

// Upload saves an object under the supplied key.
//
// The caller must validate image content and generate
// a safe, unique object key before invoking Upload.
func (storage *MinIOStorage) Upload(
	ctx context.Context,
	objectKey string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	if objectKey == "" || reader == nil || size <= 0 {
		return errors.New(
			"invalid object upload parameters",
		)
	}

	_, err := storage.client.PutObject(
		ctx,
		storage.bucket,
		objectKey,
		reader,
		size,
		minio.PutObjectOptions{
			ContentType: contentType,
		},
	)

	if err != nil {
		return safeStorageError("upload object to MinIO", err)
	}

	return nil
}

// Download retrieves a private object.
//
// The returned reader must be closed by the caller.
// Reading may still produce an error if the object is
// missing or the storage connection fails.
func (storage *MinIOStorage) Download(
	ctx context.Context,
	objectKey string,
) (io.ReadCloser, error) {
	if objectKey == "" {
		return nil, errors.New(
			"object key is required",
		)
	}

	object, err := storage.client.GetObject(
		ctx,
		storage.bucket,
		objectKey,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, safeStorageError("retrieve object from MinIO", err)
	}

	// GetObject may return a reader before a missing-object
	// or permission error becomes apparent.
	if _, err := object.Stat(); err != nil {
		_ = object.Close()

		return nil, safeStorageError("inspect MinIO object", err)
	}

	return object, nil
}

// Delete removes an object from the private bucket.
func (storage *MinIOStorage) Delete(
	ctx context.Context,
	objectKey string,
) error {
	if objectKey == "" {
		return errors.New(
			"object key is required",
		)
	}

	err := storage.client.RemoveObject(
		ctx,
		storage.bucket,
		objectKey,
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		return safeStorageError("delete MinIO object", err)
	}

	return nil
}
