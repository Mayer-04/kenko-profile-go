package historypdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	// Asegura la zona America/Bogota en contenedores sin zoneinfo (distroless).
	_ "time/tzdata"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/go-pdf/fpdf"
)

//go:embed assets/kenko_limpio.png
var logoPNG []byte

// Generator crea el PDF del historial médico con fpdf (Go puro).
type Generator struct{}

// NewGenerator construye el generador.
func NewGenerator() *Generator { return &Generator{} }

// Generate devuelve los bytes del PDF (empiezan con %PDF).
func (g *Generator) Generate(snapshot documents.ProfileSnapshot, appointments []documents.CompletedAppointment, exp documents.MedicalHistoryExport) ([]byte, error) {
	pdfDoc := fpdf.New("P", "mm", "A4", "")
	// Sin compresión para que el texto sea verificable en tests.
	pdfDoc.SetCompression(false)
	// Fuente core con traductor cp1252 para tildes y ñ.
	tr := pdfDoc.UnicodeTranslatorFromDescriptor("cp1252")
	pdfDoc.SetTitle(tr("Historial médico"), false)
	pdfDoc.AddPage()
	// Logo en la esquina superior derecha (mismo estilo que el certificado).
	if len(logoPNG) > 0 {
		imgName := "kenko_logo"
		pdfDoc.RegisterImageOptionsReader(imgName,
			fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(logoPNG))
		pdfDoc.Image(imgName, 170, 10, 30, 0, false, "PNG", 0, "")
	}
	pdfDoc.SetFont("Arial", "B", 18)
	pdfDoc.CellFormat(0, 12, tr("Historial médico"), "", 1, "C", false, 0, "")
	pdfDoc.Ln(4)

	pdfDoc.SetFont("Arial", "", 12)
	fullName := strings.TrimSpace(strings.TrimSpace(snapshot.FirstName) + " " + strings.TrimSpace(snapshot.LastName))
	header := []string{
		"Nombre: " + fullName,
		"Tipo de documento: " + snapshot.DocumentType,
		"Número de documento: " + snapshot.DocumentNumber,
		"Generado: " + formatBogota(exp.GeneratedAt),
		fmt.Sprintf("Citas incluidas: %d", len(appointments)),
	}
	for _, l := range header {
		pdfDoc.CellFormat(0, 8, tr(l), "", 1, "L", false, 0, "")
	}
	pdfDoc.Ln(4)

	for _, a := range appointments {
		pdfDoc.SetFont("Arial", "B", 12)
		pdfDoc.CellFormat(0, 8, tr(formatDay(a.Date)+" - "+a.Type.SpanishLabel()), "", 1, "L", false, 0, "")
		pdfDoc.SetFont("Arial", "", 11)
		lines := []string{
			"Hora: " + a.StartTime + " - " + a.EndTime,
			"Médico: " + a.Physician.Name,
			"Sede: " + a.MedicalCenter.Name + " (" + a.MedicalCenter.Location + ")",
			"Notas: " + a.Notes,
		}
		for _, l := range lines {
			pdfDoc.MultiCell(0, 7, tr(l), "", "L", false)
		}
		pdfDoc.Ln(2)
	}

	pdfDoc.SetFont("Arial", "I", 10)
	pdfDoc.CellFormat(0, 8, tr("Documento generado automáticamente. Datos ficticios con fines académicos."), "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdfDoc.Output(&buf); err != nil {
		return nil, fmt.Errorf("genera pdf historial: %w", err)
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

// formatDay convierte "YYYY-MM-DD" a "dd/mm/YYYY" para mostrar.
// No convierte zona horaria: el día ya es calendario.
func formatDay(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("02/01/2006")
}
