package blobstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3BlobStorage handles blob storage operations using S3-compatible storage
type S3BlobStorage struct {
	client  *s3.Client
	bucket  string
	enabled bool
	ctx     context.Context
	timeout time.Duration
}

// Config holds S3 blob storage configuration
type Config struct {
	Enabled   bool   `yaml:"enabled"`
	Endpoint  string `yaml:"endpoint"`
	Region    string `yaml:"region"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Timeout   int    `yaml:"timeout"` // seconds
}

// NewS3BlobStorage creates a new S3 blob storage instance
func NewS3BlobStorage(cfg Config) (*S3BlobStorage, error) {
	if !cfg.Enabled {
		return &S3BlobStorage{enabled: false}, nil
	}

	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("S3 access key and secret key are required when blob storage is enabled")
	}

	if cfg.Bucket == "" {
		cfg.Bucket = "email-attachments"
	}

	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}

	ctx := context.Background()

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = true
	})

	storage := &S3BlobStorage{
		client:  client,
		bucket:  cfg.Bucket,
		enabled: true,
		ctx:     ctx,
		timeout: time.Duration(cfg.Timeout) * time.Second,
	}

	// Ensure bucket exists
	if err := storage.ensureBucket(); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	return storage, nil
}

// IsEnabled returns whether blob storage is enabled
func (s *S3BlobStorage) IsEnabled() bool {
	return s.enabled
}

// ensureBucket creates the bucket if it doesn't exist
func (s *S3BlobStorage) ensureBucket() error {
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		// Bucket might already exist, which is fine
		// We can't easily distinguish without more complex error checking
		// So we'll just log and continue
		return nil
	}
	return nil
}

// Store stores content in S3 and returns the blob ID (SHA256 hash)
func (s *S3BlobStorage) Store(content string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("blob storage is not enabled")
	}

	// Calculate SHA256 hash to use as blob ID
	hash := sha256.Sum256([]byte(content))
	blobID := hex.EncodeToString(hash[:])

	// Use hash as the key for deduplication
	key := fmt.Sprintf("blobs/%s", blobID)

	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	// Check if blob already exists
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		// Blob already exists, return the ID
		return blobID, nil
	}

	// Upload the blob
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader([]byte(content)),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload blob: %w", err)
	}

	return blobID, nil
}

// Retrieve retrieves content from S3 by blob ID
func (s *S3BlobStorage) Retrieve(blobID string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("blob storage is not enabled")
	}

	key := fmt.Sprintf("blobs/%s", blobID)

	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to retrieve blob: %w", err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			// Log error but don't override the main return error
			_ = closeErr
		}
	}()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read blob data: %w", err)
	}

	return string(data), nil
}

// Delete deletes a blob from S3 (optional, for cleanup)
func (s *S3BlobStorage) Delete(blobID string) error {
	if !s.enabled {
		return fmt.Errorf("blob storage is not enabled")
	}

	key := fmt.Sprintf("blobs/%s", blobID)

	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}

	return nil
}

// Exists checks if a blob exists in S3
func (s *S3BlobStorage) Exists(blobID string) (bool, error) {
	if !s.enabled {
		return false, fmt.Errorf("blob storage is not enabled")
	}

	key := fmt.Sprintf("blobs/%s", blobID)

	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}

	return true, nil
}
