package documents

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func TestExecutePDFFails(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{err: errors.New("pdf roto")}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, time.Now().UTC(), []string{c1.String(), c2.String()}, &buf)
	if _, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("pdf fail debería ser ErrStorage: %v", err)
	}
	if len(repo.saved) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe guardar ni publicar si falla el pdf")
	}
}

func TestExecuteSaveFails(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{err: errors.New("mongo caído")}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, time.Now().UTC(), []string{c1.String(), c2.String()}, &buf)
	if _, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("save fail debería ser ErrStorage: %v", err)
	}
	if len(stor.keys) != 1 || len(pub.events) != 0 {
		t.Fatal("debe haber subido pero no publicado")
	}
}

func TestExecuteInvalidIDsFailValidation(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	uc := newUsecase(reader, repo, pdfGen, stor, pub, time.Now().UTC(), []string{"no-uuid", "tambien-malo"}, &buf)
	if _, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("ids inválidos deberían ser ErrStorage: %v", err)
	}
	if pdfGen.calls != 0 {
		t.Fatal("no debe generar pdf si la validación falla")
	}
}

func TestExecuteGeneratesCorrelationWhenEmpty(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	c3, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, time.Now().UTC(), []string{c1.String(), c2.String(), c3.String()}, &buf)
	_, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID, Email: "a@b.com"})
	if err != nil {
		t.Fatalf("debería pasar: %v", err)
	}
	if pub.events[0].Metadata.CorrelationID != c3.String() {
		t.Fatalf("correlation generada = %q, quiero %q", pub.events[0].Metadata.CorrelationID, c3.String())
	}
}

func TestShortIDBranches(t *testing.T) {
	if shortID("b8e4f0d2-3c5e-4b7f-9012-4d6e8f0a1c03") != "b8e4f0d2" {
		t.Fatal("con guion debería cortar")
	}
	if shortID("abcdefghij") != "abcdefgh" {
		t.Fatal("largo sin guion debería cortar a 8")
	}
	if shortID("abc") != "abc" {
		t.Fatal("corto debería quedar igual")
	}
}

func TestMaskEmailBranches(t *testing.T) {
	if maskEmail("maria.gomez@example.com") != "ma***@example.com" {
		t.Fatalf("mask normal: %q", maskEmail("maria.gomez@example.com"))
	}
	if maskEmail("ab@example.com") != "a***@example.com" {
		t.Fatalf("mask corto: %q", maskEmail("ab@example.com"))
	}
	if maskEmail("sin-arroba") != "***" {
		t.Fatal("sin @ debería ser ***")
	}
	if maskEmail("") != "***" {
		t.Fatal("vacío debería ser ***")
	}
}
