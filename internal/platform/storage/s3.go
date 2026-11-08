package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PutObjectAPI es el subconjunto del SDK usado (para fakes en tests).
type PutObjectAPI interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Storage sube PDFs de certificados.
type S3Storage struct {
	client PutObjectAPI
	bucket string
}

// NewS3Storage inyecta el cliente y el bucket de configuración.
func NewS3Storage(client PutObjectAPI, bucket string) *S3Storage {
	return &S3Storage{client: client, bucket: bucket}
}

// Upload guarda el PDF con ContentType application/pdf.
func (s *S3Storage) Upload(ctx context.Context, key string, body []byte, contentType string) error {
	if s.bucket == "" {
		return fmt.Errorf("bucket vacío")
	}
	if key == "" {
		return fmt.Errorf("key vacía")
	}
	if len(body) == 0 {
		return fmt.Errorf("cuerpo vacío")
	}
	if contentType == "" {
		contentType = "application/pdf"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put: %w", err)
	}
	return nil
}

// NewClient construye el cliente S3 desde la config AWS.
func NewClient(cfg aws.Config) *s3.Client {
	return s3.NewFromConfig(cfg)
}
