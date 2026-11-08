package storage

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakePutObject struct {
	gotBucket string
	gotKey    string
	gotCT     string
	err       error
}

func (f *fakePutObject) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.gotBucket = aws.ToString(in.Bucket)
	f.gotKey = aws.ToString(in.Key)
	f.gotCT = aws.ToString(in.ContentType)
	return &s3.PutObjectOutput{}, nil
}

func TestUploadVerifiesKeyAndContentType(t *testing.T) {
	fake := &fakePutObject{}
	st := NewS3Storage(fake, "my-bucket")
	key := "certificates/user-1/cert-1.pdf"
	if err := st.Upload(context.Background(), key, []byte("%PDF-fake"), "application/pdf"); err != nil {
		t.Fatalf("upload falló: %v", err)
	}
	if fake.gotBucket != "my-bucket" || fake.gotKey != key || fake.gotCT != "application/pdf" {
		t.Fatalf("parámetros incorrectos: %+v", fake)
	}
}
