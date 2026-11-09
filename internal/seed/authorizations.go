package seed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// authzAssignment es UNA autorización de demostración por usuario.
// Lista editable: cambia tipo, estado o motivo con facilidad.
type authzAssignment struct {
	// AppointmentType elige la cita más reciente de ese tipo.
	AppointmentType documents.AppointmentType
	Type            documents.AuthorizationType
	Status          documents.AuthorizationStatus
	RejectionReason documents.RejectionReason // solo si REJECTED
}

// DemoAuthorizations asigna UNA autorización por usuario, en orden.
// Con más usuarios que asignaciones se cicla la tabla (i % 3).
var DemoAuthorizations = []authzAssignment{
	{
		AppointmentType: documents.AppointmentTypeMedicinaGeneral,
		Type:            documents.AuthorizationTypeBloodTest,
		Status:          documents.AuthorizationStatusInProgress,
	},
	{
		AppointmentType: documents.AppointmentTypeMedicinaGeneral,
		Type:            documents.AuthorizationTypeXRay,
		Status:          documents.AuthorizationStatusRejected,
		RejectionReason: documents.RejectionReasonMissingDocumentation,
	},
	{
		AppointmentType: documents.AppointmentTypeOdontologia,
		Type:            documents.AuthorizationTypeDentalCleaning,
		Status:          documents.AuthorizationStatusApproved,
	},
}

// BuildAuthorizations arma UNA autorización por usuario a partir de sus citas.
// Elige la cita más reciente del tipo pedido; con más usuarios que
// asignaciones se cicla la tabla (usuario i usa la asignación i % 3).
// Valida existencia, titular y compatibilidad. El id es UUID v5 de
// (appointmentId, tipo): re-ejecutar actualiza.
func BuildAuthorizations(users []DemoUser, byUser map[string][]documents.CompletedAppointment, now time.Time) ([]documents.Authorization, error) {
	now = now.UTC()
	if len(DemoAuthorizations) == 0 {
		return nil, fmt.Errorf("sin asignaciones de demostración")
	}
	out := make([]documents.Authorization, 0, len(users))
	for i, u := range users {
		assign := DemoAuthorizations[i%len(DemoAuthorizations)]
		appt, err := newestAppointmentOfType(byUser[u.UserID], assign.AppointmentType)
		if err != nil {
			return nil, fmt.Errorf("usuario %d: %w", i, err)
		}
		if strings.TrimSpace(appt.UserID) != strings.TrimSpace(u.UserID) {
			return nil, fmt.Errorf("usuario %d: la cita no es del usuario", i)
		}
		createdAt := appt.CompletedAt.UTC()
		updatedAt := createdAt
		if assign.Status != documents.AuthorizationStatusInProgress {
			updatedAt = createdAt.Add(48 * time.Hour)
			if updatedAt.After(now) {
				updatedAt = now
			}
		}
		a := documents.Authorization{
			ID:              uuid.NewV5(uuid.NamespaceOID, "kenko:authorization:"+appt.ID+":"+string(assign.Type)).String(),
			UserID:          u.UserID,
			AppointmentID:   appt.ID,
			Type:            assign.Type,
			Status:          assign.Status,
			RejectionReason: assign.RejectionReason,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		}
		if err := a.ValidateForAppointment(appt); err != nil {
			return nil, fmt.Errorf("usuario %d: %w", i, err)
		}
		out = append(out, a)
	}
	return out, nil
}

// newestAppointmentOfType devuelve la cita más reciente del tipo (por date desc).
func newestAppointmentOfType(appts []documents.CompletedAppointment, typ documents.AppointmentType) (documents.CompletedAppointment, error) {
	best := -1
	for i, a := range appts {
		if a.Type != typ {
			continue
		}
		if best < 0 || a.Date > appts[best].Date ||
			(a.Date == appts[best].Date && a.StartTime > appts[best].StartTime) {
			best = i
		}
	}
	if best < 0 {
		return documents.CompletedAppointment{}, fmt.Errorf("sin cita atendida de tipo %s", typ)
	}
	return appts[best], nil
}

// SeedAuthorizations inserta las autorizaciones sin sobrescribir las
// existentes (pueden haber cambiado de estado) ni duplicarlas.
// Usa $setOnInsert con el índice único {appointmentId, type}.
func SeedAuthorizations(ctx context.Context, db *mongodriver.Database, auths []documents.Authorization, dryRun bool) (int, error) {
	coll := db.Collection(platformmongo.AuthorizationsCollection)
	set := 0
	// Orden estable para logs determinísticos.
	sort.Slice(auths, func(i, j int) bool {
		if auths[i].UserID != auths[j].UserID {
			return auths[i].UserID < auths[j].UserID
		}
		return auths[i].Type < auths[j].Type
	})
	for _, a := range auths {
		if err := a.Validate(); err != nil {
			return set, fmt.Errorf("autorización inválida: %w", err)
		}
		if dryRun {
			set++
			continue
		}
		_, err := coll.UpdateOne(ctx,
			bson.M{"appointmentId": a.AppointmentID, "type": string(a.Type)},
			bson.M{"$setOnInsert": a},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			return set, fmt.Errorf("guardar autorización: %w", err)
		}
		set++
	}
	return set, nil
}
