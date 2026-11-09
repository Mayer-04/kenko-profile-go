package documents

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func TestHistoryExecutePDFFails(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{hAppointment(snap.UserID)}}
	repo := &hFakeRepo{}
	pub := &hFakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newHistoryUsecase(reader, fa, repo, &hFakePDF{err: errors.New("pdf roto")}, &hFakeStorage{}, pub, time.Now().UTC(), []string{c1.String(), c2.String()}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("pdf fail debería ser ErrStorage: %v", err)
	}
	if len(repo.saved) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe guardar ni publicar si falla el pdf")
	}
}

func TestHistoryExecuteSaveFails(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{hAppointment(snap.UserID)}}
	pub := &hFakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newHistoryUsecase(reader, fa, &hFakeRepo{err: errors.New("mongo caído")}, &hFakePDF{}, &hFakeStorage{}, pub, time.Now().UTC(), []string{c1.String(), c2.String()}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("save fail debería ser ErrStorage: %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("no debe publicar si falla el guardado")
	}
}

func TestHistoryExecuteGeneratesCorrelationWhenEmpty(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{hAppointment(snap.UserID)}}
	pub := &hFakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	c3, _ := uuid.NewV7()
	uc := newHistoryUsecase(reader, fa, &hFakeRepo{}, &hFakePDF{}, &hFakeStorage{}, pub, time.Now().UTC(), []string{c1.String(), c2.String(), c3.String()}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); err != nil {
		t.Fatalf("debería pasar: %v", err)
	}
	if pub.events[0].Metadata.CorrelationID != c3.String() {
		t.Fatalf("correlation = %q, quiero %q", pub.events[0].Metadata.CorrelationID, c3.String())
	}
}
