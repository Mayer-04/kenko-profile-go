package storage

import (
	"cmp"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// TestIntegrationTransactions comprueba que el servidor de KENKO_TX_TEST_URI
// soporta transacciones multidocumento (requiere replica set, sección 8.1).
//
//	KENKO_TX_TEST_URI=mongodb://usuario:clave@127.0.0.1:27017/kenko_eps \
//		go test ./internal/profile/storage -run TestIntegrationTransactions -v
//
// Sin la variable de entorno, el test se omite.
func TestIntegrationTransactions(t *testing.T) {
	uri := os.ExpandEnv(os.Getenv("KENKO_TX_TEST_URI"))
	if uri == "" {
		t.Skip("define KENKO_TX_TEST_URI para ejecutar esta prueba de integración")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("conectando: %v", err)
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping: %v", err)
	}

	database := cmp.Or(os.Getenv("KENKO_TX_TEST_DB"), "kenko_eps")
	db := client.Database(database)
	left := db.Collection("tx_probe_left")
	right := db.Collection("tx_probe_right")
	defer func() {
		_ = left.Drop(context.Background())
		_ = right.Drop(context.Background())
	}()

	session, err := client.StartSession()
	if err != nil {
		t.Fatalf("iniciando sesión: %v", err)
	}
	defer session.EndSession(context.Background())

	_, err = session.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		if _, err := left.InsertOne(txCtx, bson.M{"n": 1}); err != nil {
			return nil, err
		}
		if _, err := right.InsertOne(txCtx, bson.M{"n": 1}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("la transacción falló: %v", err)
	}

	leftCount, err := left.CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("contando left: %v", err)
	}
	rightCount, err := right.CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("contando right: %v", err)
	}
	if leftCount != 1 || rightCount != 1 {
		t.Fatalf("la transacción no persistió ambos documentos: left=%d right=%d", leftCount, rightCount)
	}
}

// TestIntegrationStore ejecuta el flujo completo del Store (perfil + outbox +
// marcador de idempotencia, con duplicado y cambio de email) contra el
// servidor de KENKO_TX_TEST_URI. Sin la variable, se omite.
func TestIntegrationStore(t *testing.T) {
	uri := os.ExpandEnv(os.Getenv("KENKO_TX_TEST_URI"))
	if uri == "" {
		t.Skip("define KENKO_TX_TEST_URI para ejecutar esta prueba de integración")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("conectando: %v", err)
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Usa la base real de la URI: el usuario de aplicación solo tiene permisos
	// sobre ella. Las colecciones son las del servicio (EnsureSchema es
	// idempotente) y al final se borra solo lo creado por este test.
	database := cmp.Or(os.Getenv("KENKO_TX_TEST_DB"), "kenko_eps")
	store := NewStore(client, database)
	if err := store.EnsureSchema(ctx, 7); err != nil {
		t.Fatalf("preparando el esquema: %v", err)
	}

	p := testProfile()
	marker := profile.ProcessedEvent{
		ID:          uuid.NewV7(),
		EventID:     uuid.NewV7(),
		ProcessedAt: time.Now().UTC(),
	}
	emailMarker := profile.ProcessedEvent{ID: uuid.NewV7(), EventID: uuid.NewV7(), ProcessedAt: time.Now().UTC()}
	defer cleanupStore(t, store, p.ID, marker.EventID, emailMarker.EventID)

	out := profile.OutboxEvent{
		ID:            uuid.NewV7(),
		EventType:     profile.TypeProfileCreated,
		AggregateID:   p.ID,
		CorrelationID: "01a136f6-7a11-7b22-c3d4-e5f60718293a",
		Payload:       `{"metadata":{"eventId":"x"},"data":{}}`,
		CreatedAt:     time.Now().UTC(),
	}

	if err := store.CreateProfile(ctx, p, out, marker); err != nil {
		t.Fatalf("creando el perfil: %v", err)
	}

	// El mismo evento otra vez: duplicado, sin crear nada nuevo.
	err = store.CreateProfile(ctx, p, out, marker)
	if !errors.Is(err, profile.ErrDuplicateEvent) {
		t.Fatalf("reprocesando el mismo evento, error = %v, se quiere ErrDuplicateEvent", err)
	}

	got, err := store.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatalf("consultando el perfil: %v", err)
	}
	if got.Email != p.Email || got.Phone != p.Phone {
		t.Errorf("perfil guardado = %+v", got)
	}

	// Cambio de email más reciente: se aplica.
	change := profile.EmailChange{
		UserID:        p.ID,
		Email:         "nueva@example.com",
		EmailVerified: true,
		OccurredAt:    time.Now().UTC(),
	}
	applied, err := store.ApplyEmailChange(ctx, change, emailMarker)
	if err != nil || !applied {
		t.Fatalf("aplicando cambio de email: applied=%v error=%v", applied, err)
	}

	// El mismo cambio otra vez: duplicado.
	applied, err = store.ApplyEmailChange(ctx, change, emailMarker)
	if !errors.Is(err, profile.ErrDuplicateEvent) {
		t.Fatalf("reprocesando el cambio de email, error = %v, se quiere ErrDuplicateEvent", err)
	}
	if applied {
		t.Error("un duplicado no debe aplicar el cambio")
	}

	got, err = store.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatalf("consultando el perfil tras el cambio: %v", err)
	}
	if got.Email != "nueva@example.com" || !got.EmailVerified {
		t.Errorf("email tras el cambio = %q verificado=%v", got.Email, got.EmailVerified)
	}

	// PATCH: se actualiza phone y address.
	phone := "+573002223344"
	address := &profile.Address{Line: "Calle 100", City: "Medellin"}
	updated, err := store.UpdateProfile(ctx, p.ID, profile.Patch{Phone: &phone, Address: address})
	if err != nil {
		t.Fatalf("aplicando el PATCH: %v", err)
	}
	if updated.Phone != phone || updated.AddressLine != "Calle 100" || updated.City != "Medellin" {
		t.Errorf("perfil actualizado = %+v", updated)
	}
}

// cleanupStore borra los documentos que el test creó en las colecciones reales.
func cleanupStore(t *testing.T, store *Store, profileID uuid.UUID, eventIDs ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := store.profiles.DeleteOne(ctx, bson.M{"_id": binaryUUID(profileID)}); err != nil {
		t.Logf("limpiando profiles: %v", err)
	}
	if _, err := store.outbox.DeleteMany(ctx, bson.M{"aggregate_id": binaryUUID(profileID)}); err != nil {
		t.Logf("limpiando outbox_events: %v", err)
	}
	for _, eventID := range eventIDs {
		if _, err := store.processed.DeleteMany(ctx, bson.M{"event_id": binaryUUID(eventID)}); err != nil {
			t.Logf("limpiando processed_events: %v", err)
		}
	}
}

func testProfile() profile.Profile {
	return profile.Profile{
		ID:             uuid.NewV7(),
		DocumentType:   "CC",
		DocumentNumber: "1020304050",
		FirstName:      "Maria Fernanda",
		LastName:       "Gomez Rojas",
		BirthDate:      time.Date(1995, 3, 21, 0, 0, 0, 0, time.UTC),
		Gender:         "female",
		Email:          "maria.gomez@example.com",
		EmailVerified:  false,
		Phone:          "+573001234567",
		AddressLine:    "Cra 15 # 93-60",
		City:           "Bogota",
		EmailUpdatedAt: time.Date(2026, 10, 13, 20, 15, 30, 0, time.UTC),
		CreatedAt:      time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC),
	}
}
