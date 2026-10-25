// Package sqs implementa el consumo de la cola de Profile con el SDK de AWS
// (sección 5.1).
package sqs

import (
	"context"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/Mayer-04/kenko-profile-go/internal/worker"
)

// Consumer lee la cola principal del servicio. Implementa worker.Queue.
type Consumer struct {
	client      *awssqs.Client
	queueURL    string
	maxMessages int32
	waitSeconds int32
}

// NewConsumer crea el consumidor de la cola con los parámetros de la sección
// 5.1: lotes de hasta maxMessages y long polling de hasta waitSeconds.
func NewConsumer(client *awssqs.Client, queueURL string, maxMessages, waitSeconds int32) *Consumer {
	return &Consumer{
		client:      client,
		queueURL:    queueURL,
		maxMessages: maxMessages,
		waitSeconds: waitSeconds,
	}
}

// Receive trae hasta maxMessages mensajes con long polling.
func (c *Consumer) Receive(ctx context.Context) ([]worker.Message, error) {
	input := &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: c.maxMessages,
		WaitTimeSeconds:     c.waitSeconds,
	}
	output, err := c.client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, err
	}

	messages := []worker.Message{}
	for _, m := range output.Messages {
		messages = append(messages, worker.Message{
			MessageID:     aws.ToString(m.MessageId),
			ReceiptHandle: aws.ToString(m.ReceiptHandle),
			Body:          aws.ToString(m.Body),
		})
	}
	return messages, nil
}

// Delete elimina un mensaje procesado correctamente.
func (c *Consumer) Delete(ctx context.Context, receiptHandle string) error {
	input := &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	}
	_, err := c.client.DeleteMessage(ctx, input)
	return err
}

// ExtendVisibility amplía el tiempo de invisibilidad de un mensaje.
func (c *Consumer) ExtendVisibility(ctx context.Context, receiptHandle string, seconds int32) error {
	input := &awssqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(c.queueURL),
		ReceiptHandle:     aws.String(receiptHandle),
		VisibilityTimeout: seconds,
	}
	_, err := c.client.ChangeMessageVisibility(ctx, input)
	return err
}
