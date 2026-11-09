package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	// Asegura la zona America/Bogota en contenedores sin zoneinfo (distroless).
	_ "time/tzdata"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	"github.com/go-pdf/fpdf"
)

//go:embed assets/kenko_limpio.png
var logoPNG []byte

// Generator crea el PDF del certificado con fpdf (Go puro).
type Generator struct{}

// NewGenerator construye el generador.
func NewGenerator() *Generator { return &Generator{} }

// Generate devuelve los bytes del PDF (empiezan con %PDF).
func (g *Generator) Generate(snapshot documents.ProfileSnapshot, cert documents.Certificate) ([]byte, error) {
	pdfDoc := fpdf.New("P", "mm", "A4", "")
	// Sin compresión para que el texto sea verificable en tests.
	pdfDoc.SetCompression(false)
	// Fuente core con traductor cp1252 para tildes y ñ.
	tr := pdfDoc.UnicodeTranslatorFromDescriptor("cp1252")
	pdfDoc.SetTitle(tr("Certificado de afiliación"), false)
	pdfDoc.AddPage()
	// Logo en la esquina superior derecha.
	if len(logoPNG) > 0 {
		imgName := "kenko_logo"
		pdfDoc.RegisterImageOptionsReader(imgName,
			fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(logoPNG))
		// A4 210mm, margen 10mm: logo 30mm de ancho a la derecha.
		pdfDoc.Image(imgName, 170, 10, 30, 0, false, "PNG", 0, "")
	}
	pdfDoc.SetFont("Arial", "B", 18)
	pdfDoc.CellFormat(0, 12, tr("Certificado de afiliación"), "", 1, "C", false, 0, "")
	pdfDoc.Ln(6)

	pdfDoc.SetFont("Arial", "", 12)
	fullName := strings.TrimSpace(strings.TrimSpace(snapshot.FirstName) + " " + strings.TrimSpace(snapshot.LastName))
	lines := []string{
		"Nombre: " + fullName,
		"Tipo de documento: " + snapshot.DocumentType,
		"Número de documento: " + snapshot.DocumentNumber,
		"Afiliado desde: " + formatBogota(snapshot.AffiliationStartDate),
		"Fecha de emisión: " + formatBogota(cert.IssuedAt),
		"Identificador del certificado: " + cert.ID,
	}
	for _, l := range lines {
		pdfDoc.CellFormat(0, 8, tr(l), "", 1, "L", false, 0, "")
	}
	pdfDoc.Ln(6)
	pdfDoc.SetFont("Arial", "I", 10)
	pdfDoc.CellFormat(0, 8, tr("Documento generado automáticamente. Datos ficticios con fines académicos."), "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdfDoc.Output(&buf); err != nil {
		return nil, fmt.Errorf("genera pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func formatBogota(t time.Time) string {
	loc, err := time.LoadLocation("America/Bogota")
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("02/01/2006")
}

// GenerateAuthorization devuelve el PDF de la autorización (mismo estilo).
func (g *Generator) GenerateAuthorization(snapshot documents.ProfileSnapshot, item authz.AuthorizationWithAppointment) ([]byte, error) {
	pdfDoc := fpdf.New("P", "mm", "A4", "")
	pdfDoc.SetCompression(false)
	tr := pdfDoc.UnicodeTranslatorFromDescriptor("cp1252")
	pdfDoc.SetTitle(tr("Autorización médica"), false)
	pdfDoc.AddPage()
	if len(logoPNG) > 0 {
		imgName := "kenko_logo"
		pdfDoc.RegisterImageOptionsReader(imgName,
			fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(logoPNG))
		pdfDoc.Image(imgName, 170, 10, 30, 0, false, "PNG", 0, "")
	}
	pdfDoc.SetFont("Arial", "B", 18)
	pdfDoc.CellFormat(0, 12, tr("Autorización médica"), "", 1, "C", false, 0, "")
	pdfDoc.Ln(4)

	pdfDoc.SetFont("Arial", "", 12)
	fullName := strings.TrimSpace(strings.TrimSpace(snapshot.FirstName) + " " + strings.TrimSpace(snapshot.LastName))
	lines := []string{
		"Identificador de la autorización: " + item.Authorization.ID,
		"Tipo de autorización: " + item.Authorization.Type.Label(),
		"Fecha de creación: " + formatBogota(item.Authorization.CreatedAt),
		"Nombre: " + fullName,
		"Tipo de documento: " + snapshot.DocumentType,
		"Número de documento: " + snapshot.DocumentNumber,
		"Correo: " + snapshot.Email,
		"Doctor: " + item.Appointment.Physician.Name,
		"Cita de origen: " + item.Appointment.Type.SpanishLabel() + ", " + item.Appointment.Date + ", " + item.Appointment.MedicalCenter.Name,
		"Estado: " + item.Authorization.Status.Label(),
	}
	for _, l := range lines {
		pdfDoc.MultiCell(0, 8, tr(l), "", "L", false)
	}
	if item.Authorization.Status == documents.AuthorizationStatusRejected {
		pdfDoc.SetFont("Arial", "B", 12)
		pdfDoc.CellFormat(0, 8, tr("Motivo del rechazo: "+item.Authorization.RejectionReason.Label()), "", 1, "L", false, 0, "")
	}
	pdfDoc.Ln(4)
	pdfDoc.SetFont("Arial", "I", 10)
	pdfDoc.CellFormat(0, 8, tr("Documento generado automáticamente. Datos ficticios con fines académicos."), "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdfDoc.Output(&buf); err != nil {
		return nil, fmt.Errorf("genera pdf autorización: %w", err)
	}
	return buf.Bytes(), nil
}
