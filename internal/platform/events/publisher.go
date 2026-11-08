package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

// PutEventsAPI es el subconjunto del SDK usado (para fakes en tests).
type PutEventsAPI interface {
	PutEvents(ctx context.Context, in *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
}

// Publisher publica eventos de Profile en EventBridge.
type Publisher struct {
	client PutEventsAPI
	bus    string
	source string
}

// NewPublisher inyecta cliente, bus y source (kenko.profile).
func NewPublisher(client PutEventsAPI, bus, source string) *Publisher {
	return &Publisher{client: client, bus: bus, source: source}
}

// Publish envía un evento con el sobre de PutEvents.
// Trata FailedEntryCount > 0 como error de publicación.
func (p *Publisher) Publish(ctx context.Context, detailType string, detail any) error {
	payload, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal detail: %w", err)
	}
	out, err := p.client.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []types.PutEventsRequestEntry{
			{
				Source:       aws.String(p.source),
				DetailType:   aws.String(detailType),
				EventBusName: aws.String(p.bus),
				Detail:       aws.String(string(payload)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("putevents: %w", err)
	}
	if out.FailedEntryCount > 0 {
		msg := ""
		if len(out.Entries) > 0 {
			msg = aws.ToString(out.Entries[0].ErrorCode) + " " + aws.ToString(out.Entries[0].ErrorMessage)
		}
		return fmt.Errorf("putevents failed entries=%d: %s", out.FailedEntryCount, msg)
	}
	for _, e := range out.Entries {
		if e.ErrorCode != nil && aws.ToString(e.ErrorCode) != "" {
			return fmt.Errorf("putevents entry failed: %s %s", aws.ToString(e.ErrorCode), aws.ToString(e.ErrorMessage))
		}
	}
	return nil
}
