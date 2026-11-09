package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

// testUsers construye 3 usuarios válidos con ids aleatorios.
func testUsers() []DemoUser {
	mails := []string{
		"jdanielrr98@gmail.com",
		"andres.20.chaves@gmail.com",
		"jupa.roqui7@hotmail.com",
	}
	users := make([]DemoUser, 0, 3)
	for i, mail := range mails {
		id, _ := uuid.NewV7()
		users = append(users, DemoUser{
			UserID:         id.String(),
			DocumentType:   "CC",
			DocumentNumber: "100000000" + string(rune('1'+i)),
			FirstName:      "Nombre",
			LastName:       "Apellido",
			BirthDate:      "1990-01-01",
			Gender:         "female",
			Email:          mail,
			Phone:          "+573000000000",
			AddressLine:    "Calle 1",
			City:           "Bogota",
			CreatedAt:      time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
		})
	}
	return users
}

func TestGenerateAppointmentsDeterministic(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	u := testUsers()[0]
	first := GenerateAppointments(u, now)
	second := GenerateAppointments(u, now)

	if len(first) != len(documents.AppointmentTypes)*AppointmentsPerType {
		t.Fatalf("citas = %d, quiero %d", len(first), len(documents.AppointmentTypes)*AppointmentsPerType)
	}
	ids := map[string]bool{}
	for i, a := range first {
		if a.ID != second[i].ID {
			t.Fatalf("id no determinista en %d: %s vs %s", i, a.ID, second[i].ID)
		}
		if ids[a.ID] {
			t.Fatalf("id duplicado: %s", a.ID)
		}
		ids[a.ID] = true
		if err := a.Validate(); err != nil {
			t.Fatalf("cita %d inválida: %v", i, err)
		}
		day, _ := time.Parse("2006-01-02", a.Date)
		if days := now.Sub(day).Hours() / 24; days < 0 || days > 366 {
			t.Fatalf("cita %d fuera de los últimos 12 meses: %s", i, a.Date)
		}
		if a.UserID != u.UserID || a.Status != "COMPLETED" {
			t.Fatalf("cita %d con userId o status mal", i)
		}
	}
}

func TestGenerateAppointmentsCoversAllTypes(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, u := range testUsers() {
		got := GenerateAppointments(u, now)
		counts := map[documents.AppointmentType]int{}
		for _, a := range got {
			counts[a.Type]++
		}
		for _, typ := range documents.AppointmentTypes {
			if counts[typ] != AppointmentsPerType {
				t.Fatalf("usuario %s tipo %s = %d, quiero %d", u.UserID, typ, counts[typ], AppointmentsPerType)
			}
		}
	}
}

func TestGenerateAppointmentsNoFutureNoWeekend(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) // domingo
	loc, _ := time.LoadLocation("America/Bogota")
	for _, u := range testUsers() {
		seen := map[string]bool{}
		for _, a := range GenerateAppointments(u, now) {
			if a.CompletedAt.After(now) {
				t.Fatalf("cita futura: %s %s", a.Date, a.StartTime)
			}
			day, _ := time.ParseInLocation("2006-01-02", a.Date, loc)
			if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
				t.Fatalf("cita en fin de semana: %s", a.Date)
			}
			key := a.Date + "|" + a.StartTime
			if seen[key] {
				t.Fatalf("día y hora repetidos: %s", key)
			}
			seen[key] = true
		}
	}
}

func TestGenerateAppointmentsTimesAndDurations(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	want := map[documents.AppointmentType]int{
		documents.AppointmentTypeMedicinaGeneral: 30,
		documents.AppointmentTypeOdontologia:     45,
		documents.AppointmentTypePsicologia:      60,
	}
	for _, a := range GenerateAppointments(testUsers()[0], now) {
		start, err := time.Parse("15:04", a.StartTime)
		if err != nil {
			t.Fatalf("start inválido: %q", a.StartTime)
		}
		end, err := time.Parse("15:04", a.EndTime)
		if err != nil {
			t.Fatalf("end inválido: %q", a.EndTime)
		}
		if got := int(end.Sub(start).Minutes()); got != want[a.Type] {
			t.Fatalf("tipo %s duración = %d, quiero %d", a.Type, got, want[a.Type])
		}
		if a.StartTime < "08:00" || a.EndTime > "17:00" {
			t.Fatalf("fuera de horario 08:00-17:00: %s-%s", a.StartTime, a.EndTime)
		}
		if a.Physician.PhysicianID == "" || a.MedicalCenter.MedicalCenterID == "" {
			t.Fatal("médico o sede sin id")
		}
		if strings.TrimSpace(a.Notes) == "" {
			t.Fatal("notes vacías")
		}
	}
}

func TestGenerateAppointmentsCompletedAtMatchesEnd(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	loc, err := time.LoadLocation("America/Bogota")
	if err != nil {
		t.Skip("sin zona America/Bogota")
	}
	for _, a := range GenerateAppointments(testUsers()[0], now) {
		want, err := time.ParseInLocation("2006-01-02 15:04", a.Date+" "+a.EndTime, loc)
		if err != nil {
			t.Fatal(err)
		}
		if !a.CompletedAt.Equal(want.UTC()) {
			t.Fatalf("completedAt = %s, quiero fin %s UTC", a.CompletedAt, want.UTC())
		}
	}
}

func TestGenerateAppointmentsStableIDs(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	users := testUsers()
	byPhysician := map[string]string{}
	centers := map[string]bool{}
	for _, u := range users {
		for _, a := range GenerateAppointments(u, now) {
			if prev, ok := byPhysician[a.Physician.Name]; ok && prev != a.Physician.PhysicianID {
				t.Fatalf("médico %q con ids distintos", a.Physician.Name)
			}
			byPhysician[a.Physician.Name] = a.Physician.PhysicianID
			centers[a.MedicalCenter.MedicalCenterID] = true
		}
	}
	if len(centers) != 2 {
		t.Fatalf("sedes distintas = %d, quiero 2", len(centers))
	}
}

func TestAppointmentTypeSpanishLabel(t *testing.T) {
	cases := map[documents.AppointmentType]string{
		documents.AppointmentTypeMedicinaGeneral: "Medicina general",
		documents.AppointmentTypeOdontologia:     "Odontología",
		documents.AppointmentTypePsicologia:      "Psicología",
	}
	for typ, want := range cases {
		if got := typ.SpanishLabel(); got != want {
			t.Fatalf("etiqueta %s = %q, quiero %q", typ, got, want)
		}
	}
}

func TestGenerateAppointmentsVaryPerUser(t *testing.T) {
	now := time.Now().UTC()
	users := testUsers()
	a := GenerateAppointments(users[0], now)
	b := GenerateAppointments(users[1], now)
	if a[0].ID == b[0].ID {
		t.Fatal("distintos usuarios no pueden compartir id de cita")
	}
}

// writeUsersFile escribe usuarios en formato del JSON de demo.
func writeUsersFile(t *testing.T, users []DemoUser) string {
	t.Helper()
	type doc struct {
		Type   string `json:"type"`
		Number string `json:"number"`
	}
	type person struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		BirthDate string `json:"birthDate"`
		Gender    string `json:"gender"`
	}
	type contact struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
	}
	type address struct {
		Line string `json:"line"`
		City string `json:"city"`
	}
	type fileUser struct {
		UserID    string  `json:"userId"`
		Document  doc     `json:"document"`
		Person    person  `json:"person"`
		Contact   contact `json:"contact"`
		Address   address `json:"address"`
		CreatedAt string  `json:"createdAt"`
	}
	out := make([]fileUser, 0, len(users))
	for _, u := range users {
		out = append(out, fileUser{
			UserID:    u.UserID,
			Document:  doc{u.DocumentType, u.DocumentNumber},
			Person:    person{u.FirstName, u.LastName, u.BirthDate, u.Gender},
			Contact:   contact{u.Email, u.Phone},
			Address:   address{u.AddressLine, u.City},
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
		})
	}
	path := filepath.Join(t.TempDir(), "users.json")
	data := "["
	for i, f := range out {
		if i > 0 {
			data += ","
		}
		// Formato mínimo sin librerías extra.
		data += `{"userId":` + quote(f.UserID) +
			`,"document":{"type":` + quote(f.Document.Type) + `,"number":` + quote(f.Document.Number) + `}` +
			`,"person":{"firstName":` + quote(f.Person.FirstName) + `,"lastName":` + quote(f.Person.LastName) +
			`,"birthDate":` + quote(f.Person.BirthDate) + `,"gender":` + quote(f.Person.Gender) + `}` +
			`,"contact":{"email":` + quote(f.Contact.Email) + `,"phone":` + quote(f.Contact.Phone) + `}` +
			`,"address":{"line":` + quote(f.Address.Line) + `,"city":` + quote(f.Address.City) + `}` +
			`,"createdAt":` + quote(f.CreatedAt) + `}`
	}
	data += "]"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestLoadUsersAcceptsThreeAllowedEmails(t *testing.T) {
	users := testUsers()
	// Mayúsculas mezcladas: la comparación no distingue.
	users[0].Email = "JDanielRR98@Gmail.com"
	users[1].Email = "ANDRES.20.CHAVES@GMAIL.COM"
	loaded, err := LoadUsers(writeUsersFile(t, users))
	if err != nil {
		t.Fatalf("rechazó correos válidos: %v", err)
	}
	if len(loaded) != 3 || loaded[0].CreatedAt.IsZero() {
		t.Fatal("debe cargar 3 usuarios con createdAt")
	}
}

func TestLoadUsersRejects(t *testing.T) {
	cases := map[string]func([]DemoUser) []DemoUser{
		"correo ajeno": func(us []DemoUser) []DemoUser {
			us[0].Email = "otro@example.com"
			return us
		},
		"cuatro usuarios": func(us []DemoUser) []DemoUser {
			return append(us, us[0])
		},
		"dos usuarios": func(us []DemoUser) []DemoUser {
			return us[:2]
		},
		"marcador sin reemplazar": func(us []DemoUser) []DemoUser {
			us[1].UserID = UserPlaceholder
			return us
		},
		"userId no UUID": func(us []DemoUser) []DemoUser {
			us[1].UserID = "no-es-uuid"
			return us
		},
		"correos repetidos": func(us []DemoUser) []DemoUser {
			us[2].Email = us[0].Email
			return us
		},
		"campo vacío": func(us []DemoUser) []DemoUser {
			us[0].LastName = ""
			return us
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadUsers(writeUsersFile(t, mutate(testUsers()))); err == nil {
				t.Fatalf("debió rechazar: %s", name)
			}
		})
	}
}
