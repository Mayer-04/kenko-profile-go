// Package worker consume la cola SQS de Profile (5.1–5.3): long polling,
// validación contra el contrato, idempotencia y DeleteMessage solo tras
// confirmar la transacción.
package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// Handler procesa un envelope ya validado.
type Handler struct {
	svc *profile.Service
	log *slog.Logger
}

// Worker sondea SQS en bucle hasta que ctx se cancela.
type Worker struct {
	sqs       *sqs.Client
	queueURL  string
	handler   *Handler
	log       *slog.Logger
	waitTime  int32
	maxMsgs   int32
	visibility int32
}

// New crea el worker con DI explícita.
func New(sqsClient *sqs.Client, queueURL string, svc *profile.Service, log *slog.Logger, waitTime, maxMsgs int32) *Worker {
	return &Worker{
		sqs:        sqsClient,
		queueURL:   queueURL,
		handler:    &Handler{svc: svc, log: log},
		log:        log,
		waitTime:   waitTime,
		maxMsgs:    maxMsgs,
		visibility: 30,
	}
}

// Run consulta con ReceiveMessage (long polling hasta 20 s) y repite
// mientras el proceso esté activo (5.2). No registra ciclos vacíos (11.3).
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker.started", "queue", queueShort(w.queueURL))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := w.pollOnce(ctx); err != nil {
			// pollOnce ya registró; pequeña pausa para no girar en caliente.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (w *Worker) pollOnce(ctx context.Context) error {
	out, err := w.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(w.queueURL),
		MaxNumberOfMessages:   w.maxMsgs,
		WaitTimeSeconds:       w.waitTime,
		VisibilityTimeout:     w.visibility,
		MessageAttributeNames: []string{"All"},
	})
	if err != nil {
		w.log.Error("worker.receive_failed", "error", err.Error())
		return err
	}
	for _, m := range out.Messages {
		w.processOne(ctx, m)
	}
	return nil
}

func (w *Worker) processOne(ctx context.Context, m types.Message) {
	env, err := events.ParseEnvelope([]byte(aws.ToString(m.Body)))
	if err != nil {
		// Mensaje inválido: no se elimina; tras los reintentos va a la DLQ (5.3).
		w.log.Warn("worker.invalid_message")
		return
	}
	switch env.DetailType {
	case "UserRegistered":
		evt, err := events.ParseUserRegistered(env.Detail)
		if err != nil {
			w.log.Warn("worker.invalid_message", "eventId", safeEventID(env.Detail))
			return
		}
		if _, _, err := w.handler.svc.RegisterFromEvent(ctx, evt); err != nil {
			w.log.Error("worker.process_failed",
				"eventId", evt.Metadata.EventID,
				"correlationId", evt.Metadata.CorrelationID)
			return
		}
	case "EmailChanged":
		evt, err := events.ParseEmailChanged(env.Detail)
		if err != nil {
			w.log.Warn("worker.invalid_message", "eventId", safeEventID(env.Detail))
			return
		}
		if err := w.handler.svc.ApplyEmailChanged(ctx, evt); err != nil {
			w.log.Error("worker.process_failed",
				"eventId", evt.Metadata.EventID,
				"correlationId", evt.Metadata.CorrelationID)
			return
		}
	default:
		// detail-type no conocido (p. ej. regla ampliada en el futuro):
		// no eliminar para que vaya a la DLQ y sea visible.
		w.log.Warn("worker.unknown_detail_type", "detailType", env.DetailType)
		return
	}
	// Éxito o terminal (duplicado/stale/no-existe): eliminar con su ReceiptHandle (5.2).
	if _, err := w.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(w.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	}); err != nil {
		w.log.Error("worker.delete_failed")
	}
}

// ExtendVisibility amplía el ocultamiento si el procesamiento lo necesita (5.1).
func (w *Worker) ExtendVisibility(ctx context.Context, receipt string, seconds int32) error {
	_, err := w.sqs.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(w.queueURL),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: seconds,
	})
	return err
}

func queueShort(url string) string {
	// Solo el nombre de la cola, nunca credenciales.
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' {
			return url[i+1:]
		}
	}
	return url
}

func safeEventID(detail []byte) string {
	var d struct {
		Metadata struct {
			EventID string `json:"eventId"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(detail, &d); err != nil {
		return "unknown"
	}
	if d.Metadata.EventID == "" {
		return "unknown"
	}
	return d.Metadata.EventID
}
