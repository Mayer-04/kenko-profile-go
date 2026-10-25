// Package eventbridge publica eventos en el bus compartido de Kenko
// (sección 7.2).
package eventbridge

import (
	"context"
	"fmt"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awseb "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"

	"github.com/Mayer-04/kenko-profile-go/internal/relay"
)

// Publisher publica eventos en el bus. Implementa relay.Publisher.
type Publisher struct {
	client  *awseb.Client
	busName string
}

// NewPublisher crea el publicador sobre el bus indicado.
func NewPublisher(client *awseb.Client, busName string) *Publisher {
	return &Publisher{client: client, busName: busName}
}

// Publish envía las entradas en un solo PutEvents y devuelve el resultado de
// cada una, en el mismo orden (garantía de la API de EventBridge).
func (p *Publisher) Publish(ctx context.Context, entries []relay.Entry) ([]relay.Result, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	inputEntries := make([]ebtypes.PutEventsRequestEntry, 0, len(entries))
	for _, entry := range entries {
		inputEntries = append(inputEntries, ebtypes.PutEventsRequestEntry{
			EventBusName: aws.String(p.busName),
			Source:       aws.String(entry.Source),
			DetailType:   aws.String(entry.DetailType),
			Detail:       aws.String(entry.Detail),
		})
	}
	output, err := p.client.PutEvents(ctx, &awseb.PutEventsInput{Entries: inputEntries})
	if err != nil {
		return nil, err
	}
	if len(output.Entries) != len(entries) {
		return nil, fmt.Errorf("PutEvents devolvió %d resultados para %d entradas", len(output.Entries), len(entries))
	}

	results := make([]relay.Result, 0, len(entries))
	for _, entry := range output.Entries {
		results = append(results, relay.Result{
			// Una entrada vacía en ErrorCode significa que EventBridge la aceptó.
			Success:      entry.ErrorCode == nil,
			ErrorCode:    aws.ToString(entry.ErrorCode),
			ErrorMessage: aws.ToString(entry.ErrorMessage),
		})
	}
	return results, nil
}
