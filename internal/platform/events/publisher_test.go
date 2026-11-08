package events

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

type fakePutEvents struct {
	gotSource string
	gotType   string
	gotBus    string
	gotDetail map[string]any
	failed    int32
	err       error
}

func (f *fakePutEvents) PutEvents(_ context.Context, in *eventbridge.PutEventsInput, _ ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	e := in.Entries[0]
	f.gotSource = aws.ToString(e.Source)
	f.gotType = aws.ToString(e.DetailType)
	f.gotBus = aws.ToString(e.EventBusName)
	var detail map[string]any
	_ = json.Unmarshal([]byte(aws.ToString(e.Detail)), &detail)
	f.gotDetail = detail
	entries := []types.PutEventsResultEntry{{EventId: aws.String("id-1")}}
	if f.failed > 0 {
		entries[0].ErrorCode = aws.String("InternalFailure")
	}
	return &eventbridge.PutEventsOutput{FailedEntryCount: f.failed, Entries: entries}, nil
}

func TestPublishEnvelope(t *testing.T) {
	fake := &fakePutEvents{}
	pub := NewPublisher(fake, "kenko-dev-bus", "kenko.profile")
	detail := map[string]any{
		"metadata": map[string]any{"eventId": "e1", "eventVersion": "1", "occurredAt": "2026-11-04T15:00:00Z", "correlationId": "c1"},
		"data":     map[string]any{"certificateId": "cert-1"},
	}
	if err := pub.Publish(context.Background(), "CertificateGenerated", detail); err != nil {
		t.Fatalf("publish falló: %v", err)
	}
	if fake.gotSource != "kenko.profile" || fake.gotType != "CertificateGenerated" || fake.gotBus != "kenko-dev-bus" {
		t.Fatalf("sobre incorrecto: %q %q %q", fake.gotSource, fake.gotType, fake.gotBus)
	}
	if fake.gotDetail["metadata"] == nil || fake.gotDetail["data"] == nil {
		t.Fatalf("detail sin metadata/data: %v", fake.gotDetail)
	}
}

func TestPublishFailedEntryCountIsError(t *testing.T) {
	fake := &fakePutEvents{failed: 1}
	pub := NewPublisher(fake, "kenko-dev-bus", "kenko.profile")
	if err := pub.Publish(context.Background(), "CertificateGenerated", map[string]any{}); err == nil {
		t.Fatal("FailedEntryCount>0 debería ser error")
	}
}
