// Package worker consume la cola SQS de Profile, valida los eventos de
// Authentication y dispara los casos de uso (sección 5).
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// Message es un mensaje recibido de SQS.
type Message struct {
	MessageID     string
	ReceiptHandle string
	Body          string
}

// Queue es el puerto de consumo de SQS (implementado por platform/sqs).
type Queue interface {
	// Receive trae hasta MaxNumberOfMessages con long polling (sección 5.1).
	Receive(ctx context.Context) ([]Message, error)
	// Delete elimina un mensaje procesado correctamente (sección 5.1).
	Delete(ctx context.Context, receiptHandle string) error
	// ExtendVisibility amplía el tiempo de invisibilidad de un mensaje
	// (sección 5.1) antes de que venza el visibility timeout.
	ExtendVisibility(ctx context.Context, receiptHandle string, seconds int32) error
}

// Profiles es la superficie del dominio que consume el worker.
type Profiles interface {
	CreateFromRegistration(ctx context.Context, detail []byte) (profile.Outcome, error)
	ApplyEmailChange(ctx context.Context, detail []byte) (profile.Outcome, error)
}

// Config tiene los parámetros de consumo del worker (opción A de D7).
type Config struct {
	// VisibilitySeconds es el visibility timeout de la cola.
	VisibilitySeconds int32
	// RetryDelay es la espera tras un fallo al recibir mensajes.
	RetryDelay time.Duration
}

// Worker consume la cola de Profile y procesa los eventos de Authentication.
type Worker struct {
	queue    Queue
	profiles Profiles
	logger   *slog.Logger
	cfg      Config
}

// NewWorker crea el worker con sus dependencias inyectadas.
func NewWorker(queue Queue, profiles Profiles, logger *slog.Logger, cfg Config) *Worker {
	return &Worker{queue: queue, profiles: profiles, logger: logger, cfg: cfg}
}

// Run consume la cola hasta que el contexto se cancela (sección 5.2).
func (w *Worker) Run(ctx context.Context) error {
	for {
		if err := w.receiveBatch(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			w.logger.Error("fallo al recibir mensajes de SQS", "error", err)
			if !sleep(ctx, w.cfg.RetryDelay) {
				return nil
			}
		}
	}
}

// receiveBatch recibe un lote de mensajes y los procesa en orden.
func (w *Worker) receiveBatch(ctx context.Context) error {
	messages, err := w.queue.Receive(ctx)
	if err != nil {
		return err
	}

	receivedAt := time.Now()
	for _, message := range messages {
		w.handleMessage(ctx, message, time.Since(receivedAt))
		if ctx.Err() != nil {
			return nil
		}
	}
	return nil
}

// handleMessage procesa un mensaje. Solo se elimina cuando el resultado es
// correcto o un duplicado; ante cualquier fallo se deja en la cola para que
// SQS lo reintente y, tras los reintentos configurados, lo envíe a la DLQ
// (sección 5.3).
func (w *Worker) handleMessage(ctx context.Context, message Message, elapsed time.Duration) {
	// Si el lote lleva demasiado tiempo procesándose, se amplía la visibilidad
	// del mensaje antes de que venza (sección 5.1).
	if elapsed > w.visibilityGuard() {
		if err := w.queue.ExtendVisibility(ctx, message.ReceiptHandle, w.cfg.VisibilitySeconds); err != nil {
			w.logger.Warn("no se pudo ampliar la visibilidad del mensaje", "messageId", message.MessageID, "error", err)
		}
	}

	envelope, err := parseEnvelope(message.Body)
	if err != nil {
		w.logger.Warn("mensaje inválido: no es un evento de EventBridge", "messageId", message.MessageID, "error", err)
		return
	}
	if envelope.Source != profile.SourceAuthentication {
		w.logger.Warn("mensaje inválido: source inesperado", "messageId", message.MessageID, "source", envelope.Source)
		return
	}

	var (
		outcome profile.Outcome
		svcErr  error
	)
	switch envelope.DetailType {
	case profile.TypeUserRegistered:
		outcome, svcErr = w.profiles.CreateFromRegistration(ctx, []byte(envelope.Detail))
	case profile.TypeUserEmailChanged:
		outcome, svcErr = w.profiles.ApplyEmailChange(ctx, []byte(envelope.Detail))
	default:
		w.logger.Warn("mensaje inválido: detail-type desconocido", "messageId", message.MessageID, "detailType", envelope.DetailType)
		return
	}
	if svcErr != nil {
		w.logger.Error("fallo al procesar el mensaje",
			"messageId", message.MessageID,
			"detailType", envelope.DetailType,
			"error", svcErr,
		)
		return
	}

	w.logOutcome(outcome)
	if err := w.queue.Delete(ctx, message.ReceiptHandle); err != nil {
		w.logger.Error("fallo al eliminar el mensaje de SQS", "messageId", message.MessageID, "error", err)
	}
}

func (w *Worker) logOutcome(outcome profile.Outcome) {
	switch outcome.Kind {
	case profile.OutcomeCreated:
		w.logger.Info("perfil creado",
			"profileId", outcome.ProfileID,
			"eventId", outcome.EventID,
			"correlationId", outcome.CorrelationID,
		)
	case profile.OutcomeDuplicate:
		w.logger.Info("mensaje duplicado descartado",
			"eventId", outcome.EventID,
			"correlationId", outcome.CorrelationID,
		)
	case profile.OutcomeApplied:
		w.logger.Info("email del perfil actualizado",
			"profileId", outcome.ProfileID,
			"eventId", outcome.EventID,
			"correlationId", outcome.CorrelationID,
		)
	case profile.OutcomeStale:
		w.logger.Info("email ignorado por antigüedad",
			"profileId", outcome.ProfileID,
			"eventId", outcome.EventID,
			"correlationId", outcome.CorrelationID,
		)
	}
}

// visibilityGuard devuelve el tiempo a partir del cual conviene ampliar la
// visibilidad: 10 segundos antes de que venza el visibility timeout.
func (w *Worker) visibilityGuard() time.Duration {
	guard := time.Duration(w.cfg.VisibilitySeconds)*time.Second - 10*time.Second
	return max(guard, 0)
}

// eventEnvelope es la forma en minúsculas con la que EventBridge entrega los
// eventos en SQS (sección 7.1). El resto de campos del sobre (version, id,
// account, time...) se ignoran.
type eventEnvelope struct {
	Source     string         `json:"source"`
	DetailType string         `json:"detail-type"`
	Detail     jsontext.Value `json:"detail"`
}

func parseEnvelope(body string) (eventEnvelope, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return eventEnvelope{}, err
	}
	if envelope.DetailType == "" || len(envelope.Detail) == 0 {
		return eventEnvelope{}, errors.New("el evento no trae detail-type ni detail")
	}
	return envelope, nil
}

// sleep espera d salvo que el contexto se cancele antes. Devuelve false si el
// contexto ya terminó.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
