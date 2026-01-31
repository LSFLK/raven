package blobstorage

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// mockS3Client is a mock implementation of the S3 client for testing
type mockS3Client struct {
	createBucketFunc func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	putObjectFunc    func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getObjectFunc    func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	headObjectFunc   func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	deleteObjectFunc func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

func (m *mockS3Client) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	if m.createBucketFunc != nil {
		return m.createBucketFunc(ctx, params, optFns...)
	}
	return &s3.CreateBucketOutput{}, nil
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, params, optFns...)
	}
	return &s3.HeadObjectOutput{}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, params, optFns...)
	}
	return &s3.DeleteObjectOutput{}, nil
}

// Helper function to create a mock S3BlobStorage for testing
func newMockS3BlobStorage(mock S3Api, bucket string, enabled bool) *S3BlobStorage {
	return &S3BlobStorage{
		client:  mock,
		bucket:  bucket,
		enabled: enabled,
		ctx:     context.Background(),
		timeout: 30,
	}
}

func TestNewS3BlobStorage(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "disabled blob storage",
			config: Config{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "missing access key",
			config: Config{
				Enabled:   true,
				SecretKey: "secret",
			},
			expectError: true,
			errorMsg:    "S3 access key and secret key are required",
		},
		{
			name: "missing secret key",
			config: Config{
				Enabled:   true,
				AccessKey: "access",
			},
			expectError: true,
			errorMsg:    "S3 access key and secret key are required",
		},
		{
			name: "valid config with defaults",
			config: Config{
				Enabled:   true,
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
			},
			expectError: false,
		},
		{
			name: "valid config with custom values",
			config: Config{
				Enabled:   true,
				Endpoint:  "http://localhost:9000",
				Region:    "us-west-2",
				Bucket:    "custom-bucket",
				AccessKey: "test-access-key",
				SecretKey: "test-secret-key",
				Timeout:   60,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewS3BlobStorage(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if storage == nil {
				t.Fatal("expected storage to be non-nil")
			}

			// Verify enabled state
			if storage.IsEnabled() != tt.config.Enabled {
				t.Errorf("expected enabled=%v, got %v", tt.config.Enabled, storage.IsEnabled())
			}

			// Verify defaults were applied for enabled storage
			if tt.config.Enabled {
				expectedBucket := tt.config.Bucket
				if expectedBucket == "" {
					expectedBucket = "email-attachments"
				}
				if storage.bucket != expectedBucket {
					t.Errorf("expected bucket=%q, got %q", expectedBucket, storage.bucket)
				}
			}
		})
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "enabled storage",
			enabled: true,
		},
		{
			name:    "disabled storage",
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{}
			storage := newMockS3BlobStorage(mock, "test-bucket", tt.enabled)

			if storage.IsEnabled() != tt.enabled {
				t.Errorf("expected IsEnabled()=%v, got %v", tt.enabled, storage.IsEnabled())
			}
		})
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && bytesContains([]byte(s), []byte(substr))))
}

func bytesContains(b, subslice []byte) bool {
	if len(subslice) == 0 {
		return true
	}
	if len(b) < len(subslice) {
		return false
	}
	for i := 0; i <= len(b)-len(subslice); i++ {
		if bytesEqual(b[i:i+len(subslice)], subslice) {
			return true
		}
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// errorReader is a helper type that always returns an error on Read
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}
