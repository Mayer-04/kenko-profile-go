package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoStore implementa CertificateRepository, ProfileReader y el historial sobre MongoDB.
type MongoStore struct {
	db           *mongodriver.Database
	certificates *mongodriver.Collection
	profiles     *mongodriver.Collection
	appointments *mongodriver.Collection
	exports      *mongodriver.Collection
	authz        *mongodriver.Collection
}

// NewMongoStore construye el store sobre la conexión existente.
func NewMongoStore(c *platformmongo.Client) *MongoStore {
	return &MongoStore{
		db:           c.DB,
		certificates: c.DB.Collection(platformmongo.CertificatesCollection),
		profiles:     c.DB.Collection(platformmongo.ProfilesCollection),
		appointments: c.DB.Collection(platformmongo.CompletedAppointmentsCollection),
		exports:      c.DB.Collection(platformmongo.HistoryExportsCollection),
		authz:        c.DB.Collection(platformmongo.AuthorizationsCollection),
	}
}

type certDoc struct {
	ID                   string    `bson:"_id"`
	UserID               string    `bson:"userId"`
	AffiliationStartDate time.Time `bson:"affiliationStartDate"`
	IssuedAt             time.Time `bson:"issuedAt"`
	FileKey              string    `bson:"fileKey"`
}

// Save guarda el registro del certificado.
func (s *MongoStore) Save(ctx context.Context, cert documents.Certificate) error {
	doc := certDoc{
		ID:                   strings.TrimSpace(cert.ID),
		UserID:               strings.TrimSpace(cert.UserID),
		AffiliationStartDate: cert.AffiliationStartDate.UTC(),
		IssuedAt:             cert.IssuedAt.UTC(),
		FileKey:              strings.TrimSpace(cert.FileKey),
	}
	if _, err := s.certificates.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	return nil
}

// profileDoc es la proyección mínima para no importar el paquete profile.
type profileDoc struct {
	ID             bson.Binary `bson:"_id"`
	DocumentType   string      `bson:"document_type"`
	DocumentNumber string      `bson:"document_number"`
	FirstName      string      `bson:"first_name"`
	LastName       string      `bson:"last_name"`
	Email          string      `bson:"email"`
	CreatedAt      time.Time   `bson:"created_at"`
}

// GetProfile lee el perfil por userId para el certificado.
func (s *MongoStore) GetProfile(ctx context.Context, userID string) (documents.ProfileSnapshot, error) {
	userID = strings.TrimSpace(userID)
	id, err := uuid.FromString(userID)
	if err != nil {
		return documents.ProfileSnapshot{}, fmt.Errorf("%w: userId", documents.ErrProfileNotFound)
	}
	var p profileDoc
	filter := bson.M{"_id": platformmongo.UUIDToBinary(id)}
	if err := s.profiles.FindOne(ctx, filter).Decode(&p); err != nil {
		if err == mongodriver.ErrNoDocuments {
			return documents.ProfileSnapshot{}, fmt.Errorf("%w: %s", documents.ErrProfileNotFound, userID)
		}
		return documents.ProfileSnapshot{}, err
	}
	return documents.ProfileSnapshot{
		UserID:               userID,
		FirstName:            p.FirstName,
		LastName:             p.LastName,
		DocumentType:         p.DocumentType,
		DocumentNumber:       p.DocumentNumber,
		Email:                p.Email,
		AffiliationStartDate: p.CreatedAt.UTC(),
	}, nil
}

// ListAppointments lista las citas atendidas del usuario (más recientes primero).
func (s *MongoStore) ListAppointments(ctx context.Context, userID string) ([]documents.CompletedAppointment, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: userId", documents.ErrStorage)
	}
	cur, err := s.appointments.Find(ctx, bson.M{"userId": userID},
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	defer cur.Close(ctx)
	var out []documents.CompletedAppointment
	for cur.Next(ctx) {
		var a documents.CompletedAppointment
		if err := cur.Decode(&a); err != nil {
			return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
		}
		if err := a.Validate(); err != nil {
			continue
		}
		out = append(out, a)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	if out == nil {
		out = []documents.CompletedAppointment{}
	}
	return out, nil
}

type historyDoc struct {
	ID          string    `bson:"_id"`
	UserID      string    `bson:"userId"`
	RecordCount int       `bson:"recordCount"`
	FileKey     string    `bson:"fileKey"`
	GeneratedAt time.Time `bson:"generatedAt"`
}

// SaveHistoryExport guarda el registro de la exportación del historial.
func (s *MongoStore) SaveHistoryExport(ctx context.Context, exp documents.MedicalHistoryExport) error {
	doc := historyDoc{
		ID:          strings.TrimSpace(exp.ID),
		UserID:      strings.TrimSpace(exp.UserID),
		RecordCount: exp.RecordCount,
		FileKey:     strings.TrimSpace(exp.FileKey),
		GeneratedAt: exp.GeneratedAt.UTC(),
	}
	if _, err := s.exports.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	return nil
}

// ListByUser lista las autorizaciones del usuario con su cita
// (dos consultas: autorizaciones y luego citas con $in).
func (s *MongoStore) ListByUser(ctx context.Context, userID string) ([]authz.AuthorizationWithAppointment, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: userId", documents.ErrStorage)
	}
	cur, err := s.authz.Find(ctx, bson.M{"userId": userID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	defer cur.Close(ctx)
	var auths []documents.Authorization
	for cur.Next(ctx) {
		var a documents.Authorization
		if err := cur.Decode(&a); err != nil {
			return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
		}
		auths = append(auths, a)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	if len(auths) == 0 {
		return []authz.AuthorizationWithAppointment{}, nil
	}
	ids := make([]string, 0, len(auths))
	seen := map[string]bool{}
	for _, a := range auths {
		if !seen[a.AppointmentID] {
			seen[a.AppointmentID] = true
			ids = append(ids, a.AppointmentID)
		}
	}
	apptCur, err := s.appointments.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	defer apptCur.Close(ctx)
	byID := map[string]documents.CompletedAppointment{}
	for apptCur.Next(ctx) {
		var ap documents.CompletedAppointment
		if err := apptCur.Decode(&ap); err != nil {
			return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
		}
		byID[ap.ID] = ap
	}
	if err := apptCur.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	out := make([]authz.AuthorizationWithAppointment, 0, len(auths))
	for _, a := range auths {
		ap, ok := byID[a.AppointmentID]
		if !ok {
			continue
		}
		out = append(out, authz.AuthorizationWithAppointment{Authorization: a, Appointment: ap})
	}
	return out, nil
}

// GetByID obtiene una autorización del usuario con su cita.
// Si no existe o no es del usuario devuelve ErrAuthorizationNotFound.
func (s *MongoStore) GetByID(ctx context.Context, userID, authorizationID string) (authz.AuthorizationWithAppointment, error) {
	userID = strings.TrimSpace(userID)
	authorizationID = strings.TrimSpace(authorizationID)
	if userID == "" || authorizationID == "" {
		return authz.AuthorizationWithAppointment{}, fmt.Errorf("%w", documents.ErrAuthorizationNotFound)
	}
	var a documents.Authorization
	if err := s.authz.FindOne(ctx, bson.M{"_id": authorizationID, "userId": userID}).Decode(&a); err != nil {
		if err == mongodriver.ErrNoDocuments {
			return authz.AuthorizationWithAppointment{}, fmt.Errorf("%w", documents.ErrAuthorizationNotFound)
		}
		return authz.AuthorizationWithAppointment{}, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	var ap documents.CompletedAppointment
	if err := s.appointments.FindOne(ctx, bson.M{"_id": a.AppointmentID}).Decode(&ap); err != nil {
		return authz.AuthorizationWithAppointment{}, fmt.Errorf("%w: cita de la autorización: %v", documents.ErrStorage, err)
	}
	return authz.AuthorizationWithAppointment{Authorization: a, Appointment: ap}, nil
}

// SetAuthorizationFileKey guarda la clave S3 del PDF en la autorización.
func (s *MongoStore) SetAuthorizationFileKey(ctx context.Context, userID, authorizationID, fileKey string) error {
	userID = strings.TrimSpace(userID)
	authorizationID = strings.TrimSpace(authorizationID)
	fileKey = strings.TrimSpace(fileKey)
	if userID == "" || authorizationID == "" || fileKey == "" {
		return fmt.Errorf("%w: datos", documents.ErrStorage)
	}
	res, err := s.authz.UpdateOne(ctx,
		bson.M{"_id": authorizationID, "userId": userID},
		bson.M{"$set": bson.M{"fileKey": fileKey}})
	if err != nil {
		return fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w", documents.ErrAuthorizationNotFound)
	}
	return nil
}
