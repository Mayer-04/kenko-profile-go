//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testClient(t *testing.T) *platformmongo.Client {
	t.Helper()
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017/kenko_eps?directConnection=true"
	}
	client, err := mongodriver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("sin mongo local: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("sin mongo local: %v", err)
	}
	dbName := os.Getenv("MONGO_APP_DB")
	if dbName == "" {
		dbName = "kenko_eps"
	}
	c := &platformmongo.Client{Raw: client, DB: client.Database(dbName)}
	if err := c.EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("índices: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return c
}

func TestMongoStoreSaveAndGetProfile(t *testing.T) {
	c := testClient(t)
	s := NewMongoStore(c)
	ctx := context.Background()

	userID := platformmongo.MustUUID7()
	createdAt := time.Now().UTC()
	_, err := c.DB.Collection(platformmongo.ProfilesCollection).InsertOne(ctx, bson.M{
		"_id":             platformmongo.UUIDToBinary(userID),
		"document_type":   "CC",
		"document_number": "999" + userID.String()[:4],
		"first_name":      "María",
		"last_name":       "Gómez",
		"birth_date":      time.Date(1995, 3, 21, 0, 0, 0, 0, time.UTC),
		"gender":          "female",
		"email":           "maria.gomez@example.com",
		"email_verified":  false,
		"phone":           "+573001234567",
		"address_line":    "Cra 15",
		"city":            "Bogota",
		"created_at":      createdAt,
		"updated_at":      createdAt,
	})
	if err != nil {
		t.Fatalf("fixture perfil: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.DB.Collection(platformmongo.ProfilesCollection).DeleteOne(context.Background(), bson.M{"_id": platformmongo.UUIDToBinary(userID)})
	})

	snap, err := s.GetProfile(ctx, userID.String())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if snap.DocumentNumber == "" || !snap.AffiliationStartDate.Equal(createdAt) {
		t.Fatalf("snapshot incorrecto: %+v", snap)
	}

	certID := uuid.Must(uuid.NewV7())
	cert := documents.Certificate{
		ID:                   certID.String(),
		UserID:               userID.String(),
		AffiliationStartDate: snap.AffiliationStartDate,
		IssuedAt:             time.Now().UTC(),
		FileKey:              "certificates/" + userID.String() + "/" + certID.String() + ".pdf",
	}
	if err := s.Save(ctx, cert); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.DB.Collection(platformmongo.CertificatesCollection).DeleteOne(context.Background(), bson.M{"_id": certID.String()})
	})

	if _, err := s.GetProfile(ctx, uuid.Must(uuid.NewV7()).String()); err == nil {
		t.Fatal("perfil inexistente debería fallar")
	}
}
