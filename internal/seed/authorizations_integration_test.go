//go:build integration

package seed

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

func testDB(t *testing.T) *platformmongo.Client {
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

func TestSeedAuthorizationsIdempotentAndUnique(t *testing.T) {
	c := testDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	uid, _ := uuid.NewV7()
	user := DemoUser{
		UserID: uuid.Must(uuid.NewV7()).String(), DocumentType: "CC",
		DocumentNumber: "1000000001", FirstName: "Camila", LastName: "Torres Vega",
		BirthDate: "1995-03-21", Gender: "female", Email: "jdanielrr98@gmail.com",
		Phone: "+573000000001", AddressLine: "Calle 1", City: "Bogota",
		CreatedAt: time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
	}
	_ = uid
	users := []DemoUser{user,
		{
			UserID: uuid.Must(uuid.NewV7()).String(), DocumentType: "CC",
			DocumentNumber: "1000000002", FirstName: "Mateo", LastName: "Salazar Ruiz",
			BirthDate: "1992-07-08", Gender: "male", Email: "andres.20.chaves@gmail.com",
			Phone: "+573000000002", AddressLine: "Calle 2", City: "Bogota",
			CreatedAt: time.Date(2026, 4, 2, 15, 0, 0, 0, time.UTC),
		},
		{
			UserID: uuid.Must(uuid.NewV7()).String(), DocumentType: "CC",
			DocumentNumber: "1000000003", FirstName: "Valentina", LastName: "Rojas Pardo",
			BirthDate: "1998-11-30", Gender: "female", Email: "jupa.roqui7@hotmail.com",
			Phone: "+573000000003", AddressLine: "Av 3", City: "Bogota",
			CreatedAt: time.Date(2026, 5, 18, 15, 0, 0, 0, time.UTC),
		},
	}
	byUser := map[string][]documents.CompletedAppointment{}
	coll := c.DB.Collection(platformmongo.CompletedAppointmentsCollection)
	for _, u := range users {
		appts := GenerateAppointments(u, now)
		byUser[u.UserID] = appts
		for _, a := range appts {
			_, _ = coll.ReplaceOne(ctx, bson.M{"_id": a.ID}, a, options.Replace().SetUpsert(true))
		}
	}
	t.Cleanup(func() {
		ids := []string{}
		for _, u := range users {
			ids = append(ids, u.UserID)
		}
		_, _ = c.DB.Collection(platformmongo.AuthorizationsCollection).DeleteMany(context.Background(), bson.M{"userId": bson.M{"$in": ids}})
		_, _ = coll.DeleteMany(context.Background(), bson.M{"userId": bson.M{"$in": ids}})
	})

	auths, err := BuildAuthorizations(users, byUser, now)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if n, err := SeedAuthorizations(ctx, c.DB, auths, false); err != nil || n != 3 {
		t.Fatalf("seed = %d, %v; quiero 3", n, err)
	}
	// Segunda ejecución no duplica ni cambia el estado.
	first := auths[0]
	_, _ = c.DB.Collection(platformmongo.AuthorizationsCollection).UpdateOne(ctx,
		bson.M{"_id": first.ID}, bson.M{"$set": bson.M{"status": "APPROVED"}})
	if n, err := SeedAuthorizations(ctx, c.DB, auths, false); err != nil || n != 3 {
		t.Fatalf("re-seed = %d, %v", n, err)
	}
	var kept bson.M
	_ = c.DB.Collection(platformmongo.AuthorizationsCollection).FindOne(ctx, bson.M{"_id": first.ID}).Decode(&kept)
	if kept["status"] != "APPROVED" {
		t.Fatalf("re-seed no debe sobrescribir estado: %v", kept["status"])
	}
	// Unicidad {appointmentId, type}: duplicado directo debe fallar.
	dup := auths[1]
	if _, err := c.DB.Collection(platformmongo.AuthorizationsCollection).InsertOne(ctx, dup); err == nil {
		t.Fatal("duplicado appointmentId+type debería fallar por índice único")
	}
	// Reset no deja huérfanas: borra autorizaciones y luego citas.
	if _, err := c.DB.Collection(platformmongo.AuthorizationsCollection).DeleteMany(ctx, bson.M{"userId": bson.M{"$in": []string{users[0].UserID}}}); err != nil {
		t.Fatal(err)
	}
}
