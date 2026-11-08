package pdf

import (
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

func TestGenerateStartsWithPDF(t *testing.T) {
	certID, _ := uuid.NewV7()
	userID, _ := uuid.NewV7()
	snap := documents.ProfileSnapshot{
		UserID:               userID.String(),
		FirstName:            "María Fernanda",
		LastName:             "Gómez Rojas",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
	cert := documents.Certificate{
		ID:                   certID.String(),
		UserID:               userID.String(),
		AffiliationStartDate: snap.AffiliationStartDate,
		IssuedAt:             time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC),
		FileKey:              "certificates/" + userID.String() + "/" + certID.String() + ".pdf",
	}
	gen := NewGenerator()
	data, err := gen.Generate(snap, cert)
	if err != nil {
		t.Fatalf("generar falló: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pdf vacío")
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatalf("no empieza con %%PDF: %q", string(data[:10]))
	}
	// Contenido ASCII verificable (número, fechas, id).
	text := string(data)
	for _, want := range []string{"1020304050", "CC", certID.String(), "10/05/2024", "04/11/2026"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pdf debería contener %q", want)
		}
	}
}
