package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoStore implementa CertificateRepository y ProfileReader sobre MongoDB.
type MongoStore struct {
	db           *mongodriver.Database
	certificates *mongodriver.Collection
	profiles     *mongodriver.Collection
}

// NewMongoStore construye el store sobre la conexión existente.
func NewMongoStore(c *platformmongo.Client) *MongoStore {
	return &MongoStore{
		db:           c.DB,
		certificates: c.DB.Collection(platformmongo.CertificatesCollection),
		profiles:     c.DB.Collection(platformmongo.ProfilesCollection),
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
		AffiliationStartDate: p.CreatedAt.UTC(),
	}, nil
}
