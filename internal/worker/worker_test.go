package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// fakeQueue implementa worker.Queue registrando las operaciones.
type fakeQueue struct {
	mu       sync.Mutex
	pending  []Message
	deleted  []string
	extended []string
}

// Receive entrega los mensajes pendientes; cuando no quedan, bloquea hasta que
// el contexto se cancele (simulando el long polling).
func (q *fakeQueue) Receive(ctx context.Context) ([]Message, error) {
	q.mu.Lock()
	messages := q.pending
	q.pending = nil
	q.mu.Unlock()

	if len(messages) > 0 {
		return messages, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (q *fakeQueue) Delete(_ context.Context, receiptHandle string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.deleted = append(q.deleted, receiptHandle)
	return nil
}

func (q *fakeQueue) ExtendVisibility(_ context.Context, receiptHandle string, _ int32) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.extended = append(q.extended, receiptHandle)
	return nil
}

// fakeProfiles implementa worker.Profiles.
type fakeProfiles struct {
	createResult profile.Outcome
	createErr    error
	createCalls  [][]byte
	emailResult  profile.Outcome
	emailErr     error
	emailCalls   [][]byte
}

func (f *fakeProfiles) CreateFromRegistration(_ context.Context, detail []byte) (profile.Outcome, error) {
	f.createCalls = append(f.createCalls, detail)
	return f.createResult, f.createErr
}

func (f *fakeProfiles) ApplyEmailChange(_ context.Context, detail []byte) (profile.Outcome, error) {
	f.emailCalls = append(f.emailCalls, detail)
	return f.emailResult, f.emailErr
}

// envelopeOf construye el cuerpo de un mensaje SQS tal como lo entrega
// EventBridge (minúsculas, sección 7.1).
func envelopeOf(source, detailType, detail string) string {
	return `{"version":"0","id":"aws-id","source":"` + source +
		`","detail-type":"` + detailType + `","detail":` + detail + `}`
}

const validDetail = `{"metadata":{"eventId":"01a136f6-7882-72d6-afa9-cfb20ef454f6","eventVersion":"1","occurredAt":"2026-10-13T20:15:30Z","correlationId":"01a136f6-7a11-7b22-c3d4-e5f60718293a"},"data":{"userId":"01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012"}}`

func testWorker(t *testing.T) (*Worker, *fakeQueue, *fakeProfiles) {
	t.Helper()
	queue := &fakeQueue{}
	profiles := &fakeProfiles{}
	logger := slog.New(slog.DiscardHandler)
	w := NewWorker(queue, profiles, logger, Config{
		VisibilitySeconds: 30,
		RetryDelay:        time.Millisecond,
	})
	return w, queue, profiles
}

func TestHandleMessageEliminaTrasCrearElPerfil(t *testing.T) {
	w, queue, profiles := testWorker(t)

	profiles.createResult = profile.Outcome{Kind: profile.OutcomeCreated}
	message := Message{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserRegistered", validDetail)}

	w.handleMessage(t.Context(), message, 0)

	if len(profiles.createCalls) != 1 {
		t.Fatalf("creaciones = %d, se quiere 1", len(profiles.createCalls))
	}
	if len(queue.deleted) != 1 || queue.deleted[0] != "r1" {
		t.Errorf("deleted = %v, se quiere [r1]", queue.deleted)
	}
}

func TestHandleMessageEliminaDuplicados(t *testing.T) {
	w, queue, profiles := testWorker(t)

	profiles.createResult = profile.Outcome{Kind: profile.OutcomeDuplicate}
	message := Message{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserRegistered", validDetail)}

	w.handleMessage(t.Context(), message, 0)

	if len(queue.deleted) != 1 {
		t.Errorf("un duplicado se elimina de la cola, deleted = %v", queue.deleted)
	}
}

func TestHandleMessageErrorDelDominioNoElimina(t *testing.T) {
	w, queue, profiles := testWorker(t)

	profiles.createErr = errors.New("caída de MongoDB")
	message := Message{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserRegistered", validDetail)}

	w.handleMessage(t.Context(), message, 0)

	if len(queue.deleted) != 0 {
		t.Errorf("con fallo de procesamiento el mensaje debe seguir en la cola, deleted = %v", queue.deleted)
	}
}

func TestHandleMensajeInvalidoNoElimina(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "JSON mal formado",
			body:    `{"source":`,
			wantMsg: "mensaje inválido",
		},
		{
			name:    "source de otro servicio",
			body:    envelopeOf("kenko.documents", "UserRegistered", validDetail),
			wantMsg: "source inesperado",
		},
		{
			name:    "detail-type desconocido",
			body:    envelopeOf("kenko.auth", "UserDeleted", validDetail),
			wantMsg: "detail-type desconocido",
		},
		{
			name:    "detail vacío",
			body:    `{"source":"kenko.auth","detail-type":"UserRegistered"}`,
			wantMsg: "mensaje inválido",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w, queue, _ := testWorker(t)
			message := Message{MessageID: "1", ReceiptHandle: "r1", Body: test.body}

			w.handleMessage(t.Context(), message, 0)

			if len(queue.deleted) != 0 {
				t.Errorf("un mensaje inválido no se elimina (va a la DLQ tras reintentos), deleted = %v", queue.deleted)
			}
		})
	}
}

func TestHandleMessageEventoDeEmail(t *testing.T) {
	w, _, profiles := testWorker(t)

	profiles.emailResult = profile.Outcome{Kind: profile.OutcomeApplied}
	message := Message{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserEmailChanged", validDetail)}

	w.handleMessage(t.Context(), message, 0)

	if len(profiles.emailCalls) != 1 {
		t.Fatalf("cambios de email = %d, se quiere 1", len(profiles.emailCalls))
	}
	if len(profiles.createCalls) != 0 {
		t.Errorf("un UserEmailChanged no debe crear perfiles")
	}
}

func TestHandleMessageAmpliaVisibilidadSiElLoteVaLargo(t *testing.T) {
	w, queue, profiles := testWorker(t)

	profiles.createResult = profile.Outcome{Kind: profile.OutcomeCreated}
	message := Message{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserRegistered", validDetail)}

	// Más de visibilityGuard (30 s - 10 s = 20 s) desde la recepción del lote.
	w.handleMessage(t.Context(), message, 25*time.Second)

	if len(queue.extended) != 1 {
		t.Errorf("extended = %v, se quiere una ampliación de visibilidad", queue.extended)
	}
}

func TestRunTerminaConContextoCancelado(t *testing.T) {
	w, queue, _ := testWorker(t)
	queue.pending = []Message{
		{MessageID: "1", ReceiptHandle: "r1", Body: envelopeOf("kenko.auth", "UserRegistered", validDetail)},
	}

	ctx, cancel := context.WithCancel(t.Context())
	// La primera Receive entrega el lote; la siguiente bloquea y se cancela.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run debe terminar sin error al cancelar: %v", err)
	}
	if len(queue.deleted) != 1 {
		t.Errorf("deleted = %v", queue.deleted)
	}
}

func TestParseEnvelope(t *testing.T) {
	envelope, err := parseEnvelope(envelopeOf("kenko.auth", "UserRegistered", validDetail))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if envelope.Source != "kenko.auth" {
		t.Errorf("source = %q", envelope.Source)
	}
	if envelope.DetailType != "UserRegistered" {
		t.Errorf("detail-type = %q", envelope.DetailType)
	}
	if !strings.Contains(string(envelope.Detail), `"eventId"`) {
		t.Errorf("detail = %s", envelope.Detail)
	}
}
