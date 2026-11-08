package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	if len(first) < 4 || len(first) > 6 {
		t.Fatalf("citas fuera de rango 4-6: %d", len(first))
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
