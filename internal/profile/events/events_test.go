package events

import (
	"encoding/json"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func validDetail(t *testing.T) json.RawMessage {
	t.Helper()
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	return json.RawMessage(`{
		"metadata": {"eventId": "` + eid.String() + `", "eventVersion": "1",
			"occurredAt": "2026-10-13T20:15:30Z", "correlationId": "` + cid.String() + `"},
		"data": {"userId": "` + uid.String() + `",
			"document": {"type": "CC", "number": "1020304050"},
			"person": {"firstName": "Maria Fernanda", "lastName": "Gomez Rojas",
				"birthDate": "1995-03-21", "gender": "female"},
			"contact": {"email": "maria.gomez@example.com", "emailVerified": false, "phone": "+573001234567"},
			"address": {"line": "Cra 15 # 93-60 Apto 402", "city": "Bogota"}}
	}`)
}

func TestParseUserRegisteredValid(t *testing.T) {
	evt, err := ParseUserRegistered(validDetail(t))
	if err != nil {
		t.Fatalf("válido rechazado: %v", err)
	}
	if evt.Data.Person.FirstName != "Maria Fernanda" {
		t.Fatalf("mapeo incorrecto: %q", evt.Data.Person.FirstName)
	}
}

func TestParseUserRegisteredBadVersion(t *testing.T) {
	d := validDetail(t)
	var m map[string]any
	if err := json.Unmarshal(d, &m); err != nil {
		t.Fatal(err)
	}
	m["metadata"].(map[string]any)["eventVersion"] = "2"
	raw, _ := json.Marshal(m)
	if _, err := ParseUserRegistered(raw); err == nil {
		t.Fatal("eventVersion 2 debería rechazarse")
	}
}

func TestParseEnvelopeRejectsForeignSource(t *testing.T) {
	body := []byte(`{"source": "kenko.documents", "detail-type": "UserRegistered", "detail": {}}`)
	if _, err := ParseEnvelope(body); err == nil {
		t.Fatal("source ajena debería rechazarse")
	}
}

func TestParseEnvelopeAcceptsFullBridgeEnvelope(t *testing.T) {
	d := validDetail(t)
	body, _ := json.Marshal(map[string]any{
		"version": "0", "id": "abc", "source": "kenko.auth",
		"detail-type": "UserRegistered", "detail": json.RawMessage(d),
		"account": "137696817246", "time": "2026-10-13T20:15:30Z",
	})
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("envelope real rechazado: %v", err)
	}
	if _, err := ParseUserRegistered(env.Detail); err != nil {
		t.Fatalf("detail válido rechazado: %v", err)
	}
}

func validEmailChangedDetail(t *testing.T) json.RawMessage {
	t.Helper()
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	return json.RawMessage(`{
		"metadata": {"eventId": "` + eid.String() + `", "eventVersion": "1",
			"occurredAt": "2026-10-26T10:00:00Z", "correlationId": "` + cid.String() + `"},
		"data": {"userId": "` + uid.String() + `",
			"oldEmail": "prev@example.com", "newEmail": "next@example.com",
			"firstName": "Maria Fernanda"}
	}`)
}

func TestParseEmailChangedValid(t *testing.T) {
	evt, err := ParseEmailChanged(validEmailChangedDetail(t))
	if err != nil {
		t.Fatalf("válido rechazado: %v", err)
	}
	if evt.Data.NewEmail != "next@example.com" {
		t.Fatalf("mapeo incorrecto: %q", evt.Data.NewEmail)
	}
	if evt.Data.OldEmail != "prev@example.com" {
		t.Fatalf("mapeo incorrecto: %q", evt.Data.OldEmail)
	}
}

func TestParseEmailChangedWithoutOptional(t *testing.T) {
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	raw := json.RawMessage(`{
		"metadata": {"eventId": "` + eid.String() + `", "eventVersion": "1",
			"occurredAt": "2026-10-26T10:00:00Z", "correlationId": "` + cid.String() + `"},
		"data": {"userId": "` + uid.String() + `", "newEmail": "next@example.com"}
	}`)
	if _, err := ParseEmailChanged(raw); err != nil {
		t.Fatalf("sin opcionales debería aceptar: %v", err)
	}
}

func TestParseEmailChangedBadVersion(t *testing.T) {
	d := validEmailChangedDetail(t)
	var m map[string]any
	if err := json.Unmarshal(d, &m); err != nil {
		t.Fatal(err)
	}
	m["metadata"].(map[string]any)["eventVersion"] = "2"
	raw, _ := json.Marshal(m)
	if _, err := ParseEmailChanged(raw); err == nil {
		t.Fatal("eventVersion 2 debería rechazarse")
	}
}

func TestParseEmailChangedRejectsMissingNewEmail(t *testing.T) {
	d := validEmailChangedDetail(t)
	var m map[string]any
	if err := json.Unmarshal(d, &m); err != nil {
		t.Fatal(err)
	}
	delete(m["data"].(map[string]any), "newEmail")
	raw, _ := json.Marshal(m)
	if _, err := ParseEmailChanged(raw); err == nil {
		t.Fatal("sin newEmail debería rechazarse")
	}
}

func TestParseEmailChangedRejectsInvalidOccurredAt(t *testing.T) {
	d := validEmailChangedDetail(t)
	var m map[string]any
	if err := json.Unmarshal(d, &m); err != nil {
		t.Fatal(err)
	}
	m["metadata"].(map[string]any)["occurredAt"] = "no-fecha"
	raw, _ := json.Marshal(m)
	if _, err := ParseEmailChanged(raw); err == nil {
		t.Fatal("occurredAt inválido debería rechazarse")
	}
}

func TestParseEmailChangedRejectsOldShape(t *testing.T) {
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	raw := json.RawMessage(`{
		"metadata": {"eventId": "` + eid.String() + `", "eventVersion": "1",
			"occurredAt": "2026-10-26T10:00:00Z", "correlationId": "` + cid.String() + `"},
		"data": {"userId": "` + uid.String() + `", "email": "next@example.com",
			"emailVerified": false, "occurredAt": "2026-10-26T10:00:00Z"}
	}`)
	if _, err := ParseEmailChanged(raw); err == nil {
		t.Fatal("forma vieja UserEmailChanged debería rechazarse (falta newEmail)")
	}
}

func TestParseEnvelopeEmailChanged(t *testing.T) {
	d := validEmailChangedDetail(t)
	body, _ := json.Marshal(map[string]any{
		"version": "0", "id": "abc", "source": "kenko.auth",
		"detail-type": "EmailChanged", "detail": json.RawMessage(d),
		"account": "137696817246", "time": "2026-10-26T10:00:00Z",
	})
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("envelope real rechazado: %v", err)
	}
	if env.DetailType != "EmailChanged" {
		t.Fatalf("detail-type incorrecto: %q", env.DetailType)
	}
	if _, err := ParseEmailChanged(env.Detail); err != nil {
		t.Fatalf("detail válido rechazado: %v", err)
	}
}
