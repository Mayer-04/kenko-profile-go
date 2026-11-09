// Package seed prepara datos ficticios para la demostración: valida el
// archivo de usuarios, inserta perfiles y genera citas atendidas.
// No toca AWS, no publica eventos y nunca escribe en Atlas (ver cmd/seed).
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Marcador que debe reemplazarse por el userId real de Authentication.
const UserPlaceholder = "REEMPLAZAR_CON_USERID_DE_AUTH"

// AllowedEmails es la lista cerrada de correos de la demo (sandbox SES).
var AllowedEmails = []string{
	"jdanielrr98@gmail.com",
	"andres.20.chaves@gmail.com",
	"jupa.roqui7@hotmail.com",
}

// DemoUser es un usuario del archivo seed/demo_users.json ya validado.
type DemoUser struct {
	UserID         string
	DocumentType   string
	DocumentNumber string
	FirstName      string
	LastName       string
	BirthDate      string // YYYY-MM-DD
	Gender         string
	Email          string
	Phone          string
	AddressLine    string
	City           string
	CreatedAt      time.Time // UTC, fecha de afiliación para el certificado
}

// fileUser refleja la forma anidada del JSON (igual que el evento).
type fileUser struct {
	UserID   string `json:"userId"`
	Document struct {
		Type   string `json:"type"`
		Number string `json:"number"`
	} `json:"document"`
	Person struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		BirthDate string `json:"birthDate"`
		Gender    string `json:"gender"`
	} `json:"person"`
	Contact struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
	} `json:"contact"`
	Address struct {
		Line string `json:"line"`
		City string `json:"city"`
	} `json:"address"`
	CreatedAt string `json:"createdAt"`
}

// LoadUsers lee y valida el archivo: exactamente 3 usuarios, correos de la
// lista cerrada (sin distinguir mayúsculas), sin duplicados ni marcadores.
// Si algo falla, retorna error y el llamador no debe escribir nada.
func LoadUsers(path string) ([]DemoUser, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", path, err)
	}
	var file []fileUser
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("JSON inválido en %s: %w", path, err)
	}
	if len(file) != len(AllowedEmails) {
		return nil, fmt.Errorf("se requieren exactamente %d usuarios, hay %d", len(AllowedEmails), len(file))
	}
	users := make([]DemoUser, 0, len(file))
	seenEmails := map[string]bool{}
	seenIDs := map[string]bool{}
	for i, f := range file {
		email := strings.TrimSpace(f.Contact.Email)
		if !allowedEmail(email) {
			return nil, fmt.Errorf("usuario %d: correo %q fuera de la lista permitida", i, email)
		}
		lower := strings.ToLower(email)
		if seenEmails[lower] {
			return nil, fmt.Errorf("correo repetido: %q", email)
		}
		seenEmails[lower] = true
		userID := strings.TrimSpace(f.UserID)
		if userID == "" || userID == UserPlaceholder {
			return nil, fmt.Errorf("usuario %d: userId con marcador sin reemplazar", i)
		}
		if _, err := uuid.FromString(userID); err != nil {
			return nil, fmt.Errorf("usuario %d: userId no es UUID válido", i)
		}
		if seenIDs[userID] {
			return nil, fmt.Errorf("userId repetido: %s", userID)
		}
		seenIDs[userID] = true
		u := DemoUser{
			UserID:         userID,
			DocumentType:   strings.TrimSpace(f.Document.Type),
			DocumentNumber: strings.TrimSpace(f.Document.Number),
			FirstName:      strings.TrimSpace(f.Person.FirstName),
			LastName:       strings.TrimSpace(f.Person.LastName),
			BirthDate:      strings.TrimSpace(f.Person.BirthDate),
			Gender:         strings.TrimSpace(f.Person.Gender),
			Email:          email,
			Phone:          strings.TrimSpace(f.Contact.Phone),
			AddressLine:    strings.TrimSpace(f.Address.Line),
			City:           strings.TrimSpace(f.Address.City),
		}
		if err := validateUser(u, i); err != nil {
			return nil, err
		}
		createdAt, err := time.Parse(time.RFC3339, strings.TrimSpace(f.CreatedAt))
		if err != nil {
			return nil, fmt.Errorf("usuario %d: createdAt debe ser RFC3339 (ej. 2026-03-10T15:00:00Z)", i)
		}
		u.CreatedAt = createdAt.UTC()
		users = append(users, u)
	}
	return users, nil
}

// validateUser revisa campos obligatorios y formatos de un usuario.
func validateUser(u DemoUser, i int) error {
	for field, value := range map[string]string{
		"document.type": u.DocumentType, "document.number": u.DocumentNumber,
		"person.firstName": u.FirstName, "person.lastName": u.LastName,
		"person.birthDate": u.BirthDate, "person.gender": u.Gender,
		"contact.phone": u.Phone, "address.line": u.AddressLine,
		"address.city": u.City,
	} {
		if value == "" {
			return fmt.Errorf("usuario %d: %s obligatorio", i, field)
		}
	}
	if _, err := time.Parse("2006-01-02", u.BirthDate); err != nil {
		return fmt.Errorf("usuario %d: birthDate debe ser YYYY-MM-DD", i)
	}
	return nil
}

// allowedEmail compara sin distinguir mayúsculas ni espacios.
func allowedEmail(email string) bool {
	for _, allowed := range AllowedEmails {
		if strings.EqualFold(strings.TrimSpace(email), allowed) {
			return true
		}
	}
	return false
}

// Options controlan el comportamiento del seed.
type Options struct {
	DryRun            bool
	Reset             bool
	OverwriteProfiles bool
	// ExtraProfiles genera N perfiles ficticios de relleno además del archivo.
	ExtraProfiles int
}

// Summary cuenta lo que el seed hizo (o haría con DryRun).
type Summary struct {
	ProfilesInserted   int
	ProfilesUpdated    int
	ProfilesSkipped    int
	AppointmentsSet    int
	AppointmentsGone   int // borradas con Reset
	AuthorizationsSet  int
	AuthorizationsGone int // borradas con Reset
	ExtraProfiles      int // relleno generado además del archivo
}

// Run ejecuta el seed: valida, inserta perfiles, citas y autorizaciones.
// Con DryRun solo cuenta sin escribir. Con Reset borra primero las
// autorizaciones y luego las citas de los usuarios, y termina.
func Run(ctx context.Context, db *mongodriver.Database, users []DemoUser, opts Options, now time.Time) (Summary, error) {
	var s Summary
	profiles := db.Collection(platformmongo.ProfilesCollection)
	appointments := db.Collection(platformmongo.CompletedAppointmentsCollection)
	authorizations := db.Collection(platformmongo.AuthorizationsCollection)
	extras := MakeExtraUsers(opts.ExtraProfiles, now)
	s.ExtraProfiles = len(extras)
	allUsers := append(append([]DemoUser{}, users...), extras...)
	userIDs := make([]string, 0, len(allUsers))
	for _, u := range allUsers {
		userIDs = append(userIDs, u.UserID)
	}

	if opts.Reset {
		if opts.DryRun {
			nAuth, err := authorizations.CountDocuments(ctx, bson.M{"userId": bson.M{"$in": userIDs}})
			if err != nil {
				return s, fmt.Errorf("contar autorizaciones: %w", err)
			}
			s.AuthorizationsGone = int(nAuth)
			n, err := appointments.CountDocuments(ctx, bson.M{"userId": bson.M{"$in": userIDs}})
			if err != nil {
				return s, fmt.Errorf("contar citas: %w", err)
			}
			s.AppointmentsGone = int(n)
			return s, nil
		}
		// Primero autorizaciones para no dejar huérfanas.
		resAuth, err := authorizations.DeleteMany(ctx, bson.M{"userId": bson.M{"$in": userIDs}})
		if err != nil {
			return s, fmt.Errorf("borrar autorizaciones: %w", err)
		}
		s.AuthorizationsGone = int(resAuth.DeletedCount)
		res, err := appointments.DeleteMany(ctx, bson.M{"userId": bson.M{"$in": userIDs}})
		if err != nil {
			return s, fmt.Errorf("borrar citas: %w", err)
		}
		s.AppointmentsGone = int(res.DeletedCount)
		return s, nil
	}

	byUser := map[string][]documents.CompletedAppointment{}
	for _, u := range allUsers {
		action, err := seedProfile(ctx, profiles, u, opts)
		if err != nil {
			return s, err
		}
		switch action {
		case "inserted":
			s.ProfilesInserted++
		case "updated":
			s.ProfilesUpdated++
		default:
			s.ProfilesSkipped++
		}
		generated := GenerateAppointments(u, now.UTC())
		byUser[u.UserID] = append([]documents.CompletedAppointment{}, generated...)
		for _, a := range generated {
			if err := a.Validate(); err != nil {
				return s, fmt.Errorf("cita generada inválida: %w", err)
			}
			if opts.DryRun {
				s.AppointmentsSet++
				continue
			}
			_, err := appointments.ReplaceOne(ctx,
				bson.M{"_id": a.ID}, a,
				options.Replace().SetUpsert(true),
			)
			if err != nil {
				return s, fmt.Errorf("guardar cita: %w", err)
			}
			s.AppointmentsSet++
		}
	}
	auths, err := BuildAuthorizations(allUsers, byUser, now.UTC())
	if err != nil {
		return s, err
	}
	n, err := SeedAuthorizations(ctx, db, auths, opts.DryRun)
	if err != nil {
		return s, err
	}
	s.AuthorizationsSet = n
	return s, nil
}

// seedProfile inserta el perfil solo si no existe (o lo actualiza con
// OverwriteProfiles). Retorna inserted|updated|skipped.
func seedProfile(ctx context.Context, coll *mongodriver.Collection, u DemoUser, opts Options) (string, error) {
	id, _ := uuid.FromString(u.UserID) // ya validado en LoadUsers
	birth, _ := time.Parse("2006-01-02", u.BirthDate)
	p := profile.Profile{
		ID:             platformmongo.UUIDToBinary(id),
		DocumentType:   u.DocumentType,
		DocumentNumber: u.DocumentNumber,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		BirthDate:      birth,
		Gender:         u.Gender,
		Email:          u.Email,
		Phone:          u.Phone,
		AddressLine:    u.AddressLine,
		City:           u.City,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.CreatedAt,
	}
	var existing bson.M
	err := coll.FindOne(ctx, bson.M{"_id": p.ID}).Decode(&existing)
	if err == nil {
		if !opts.OverwriteProfiles {
			return "skipped", nil
		}
		if opts.DryRun {
			return "updated", nil
		}
		_, err := coll.ReplaceOne(ctx, bson.M{"_id": p.ID}, p)
		return "updated", err
	}
	if err != mongodriver.ErrNoDocuments {
		return "", fmt.Errorf("buscar perfil: %w", err)
	}
	if opts.DryRun {
		return "inserted", nil
	}
	_, err = coll.InsertOne(ctx, p)
	return "inserted", err
}
