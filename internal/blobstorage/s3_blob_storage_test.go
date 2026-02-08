package blobstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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
		timeout: 30 * time.Second,
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
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
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

func TestStore(t *testing.T) {
	testContent := "test content for blob storage"
	hash := sha256.Sum256([]byte(testContent))
	expectedBlobID := hex.EncodeToString(hash[:])

	tests := []struct {
		name          string
		content       string
		enabled       bool
		setupMock     func(*mockS3Client)
		expectError   bool
		errorContains string
		expectedID    string
	}{
		{
			name:          "disabled storage",
			content:       testContent,
			enabled:       false,
			setupMock:     func(m *mockS3Client) {},
			expectError:   true,
			errorContains: "blob storage is not enabled",
		},
		{
			name:    "successful first time store",
			content: testContent,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					// Simulate blob doesn't exist
					return nil, &smithy.GenericAPIError{Code: "NotFound"}
				}
				m.putObjectFunc = func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					// Verify the key format
					expectedKey := "blobs/" + expectedBlobID
					if *params.Key != expectedKey {
						t.Errorf("expected key=%q, got %q", expectedKey, *params.Key)
					}
					return &s3.PutObjectOutput{}, nil
				}
			},
			expectError: false,
			expectedID:  expectedBlobID,
		},
		{
			name:    "deduplication - blob already exists",
			content: testContent,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					// Simulate blob already exists
					return &s3.HeadObjectOutput{}, nil
				}
				m.putObjectFunc = func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					t.Error("PutObject should not be called when blob exists")
					return nil, errors.New("should not be called")
				}
			},
			expectError: false,
			expectedID:  expectedBlobID,
		},
		{
			name:    "head object failure on store",
			content: testContent,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					return nil, errors.New("head object failed")
				}
			},
			expectError:   true,
			errorContains: "head object failed",
		},
		{
			name:    "upload failure",
			content: testContent,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					return nil, &smithy.GenericAPIError{Code: "NotFound"}
				}
				m.putObjectFunc = func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					return nil, errors.New("upload failed")
				}
			},
			expectError:   true,
			errorContains: "failed to upload blob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{}
			tt.setupMock(mock)
			storage := newMockS3BlobStorage(mock, "test-bucket", tt.enabled)

			blobID, err := storage.Store(tt.content)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if blobID != tt.expectedID {
				t.Errorf("expected blobID=%q, got %q", tt.expectedID, blobID)
			}
		})
	}
}

func TestRetrieve(t *testing.T) {
	testBlobID := "abc123def456"
	testContent := "retrieved content"

	tests := []struct {
		name            string
		blobID          string
		enabled         bool
		setupMock       func(*mockS3Client)
		expectError     bool
		errorContains   string
		expectedContent string
	}{
		{
			name:          "disabled storage",
			blobID:        testBlobID,
			enabled:       false,
			setupMock:     func(m *mockS3Client) {},
			expectError:   true,
			errorContains: "blob storage is not enabled",
		},
		{
			name:    "successful retrieval",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					expectedKey := "blobs/" + testBlobID
					if *params.Key != expectedKey {
						t.Errorf("expected key=%q, got %q", expectedKey, *params.Key)
					}
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader([]byte(testContent))),
					}, nil
				}
			},
			expectError:     false,
			expectedContent: testContent,
		},
		{
			name:    "blob not found",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return nil, &smithy.GenericAPIError{Code: "NoSuchKey"}
				}
			},
			expectError:   true,
			errorContains: "failed to retrieve blob",
		},
		{
			name:    "read error",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.getObjectFunc = func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(&errorReader{err: errors.New("read failed")}),
					}, nil
				}
			},
			expectError:   true,
			errorContains: "failed to read blob data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{}
			tt.setupMock(mock)
			storage := newMockS3BlobStorage(mock, "test-bucket", tt.enabled)

			content, err := storage.Retrieve(tt.blobID)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if content != tt.expectedContent {
				t.Errorf("expected content=%q, got %q", tt.expectedContent, content)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	testBlobID := "abc123def456"

	tests := []struct {
		name          string
		blobID        string
		enabled       bool
		setupMock     func(*mockS3Client)
		expectError   bool
		errorContains string
	}{
		{
			name:          "disabled storage",
			blobID:        testBlobID,
			enabled:       false,
			setupMock:     func(m *mockS3Client) {},
			expectError:   true,
			errorContains: "blob storage is not enabled",
		},
		{
			name:    "successful deletion",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.deleteObjectFunc = func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
					expectedKey := "blobs/" + testBlobID
					if *params.Key != expectedKey {
						t.Errorf("expected key=%q, got %q", expectedKey, *params.Key)
					}
					return &s3.DeleteObjectOutput{}, nil
				}
			},
			expectError: false,
		},
		{
			name:    "deletion error",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.deleteObjectFunc = func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
					return nil, errors.New("delete failed")
				}
			},
			expectError:   true,
			errorContains: "failed to delete blob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{}
			tt.setupMock(mock)
			storage := newMockS3BlobStorage(mock, "test-bucket", tt.enabled)

			err := storage.Delete(tt.blobID)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExists(t *testing.T) {
	testBlobID := "abc123def456"

	tests := []struct {
		name          string
		blobID        string
		enabled       bool
		setupMock     func(*mockS3Client)
		expectError   bool
		errorContains string
		expectedExist bool
	}{
		{
			name:          "disabled storage",
			blobID:        testBlobID,
			enabled:       false,
			setupMock:     func(m *mockS3Client) {},
			expectError:   true,
			errorContains: "blob storage is not enabled",
		},
		{
			name:    "blob exists",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					expectedKey := "blobs/" + testBlobID
					if *params.Key != expectedKey {
						t.Errorf("expected key=%q, got %q", expectedKey, *params.Key)
					}
					return &s3.HeadObjectOutput{}, nil
				}
			},
			expectError:   false,
			expectedExist: true,
		},
		{
			name:    "blob does not exist",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					return nil, &smithy.GenericAPIError{Code: "NotFound"}
				}
			},
			expectError:   false,
			expectedExist: false,
		},
		{
			name:    "head object error",
			blobID:  testBlobID,
			enabled: true,
			setupMock: func(m *mockS3Client) {
				m.headObjectFunc = func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					return nil, errors.New("transient network error")
				}
			},
			expectError:   true,
			errorContains: "transient network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{}
			tt.setupMock(mock)
			storage := newMockS3BlobStorage(mock, "test-bucket", tt.enabled)

			exists, err := storage.Exists(tt.blobID)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if exists != tt.expectedExist {
				t.Errorf("expected exists=%v, got %v", tt.expectedExist, exists)
			}
		})
	}
}

// errorReader is a helper type that always returns an error on Read
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}
