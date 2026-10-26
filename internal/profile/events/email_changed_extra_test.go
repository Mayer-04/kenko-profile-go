package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func emailChangedRaw(t *testing.T, mutate func(map[string]any)) json.RawMessage {
	t.Helper()
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	base := map[string]any{
		"metadata": map[string]any{
			"eventId":       eid.String(),
			"eventVersion":  "1",
			"occurredAt":    "2026-10-26T10:00:00Z",
			"correlationId": cid.String(),
		},
		"data": map[string]any{
			"userId":    uid.String(),
			"oldEmail":  "prev@example.com",
			"newEmail":  "next@example.com",
			"firstName": "Maria Fernanda",
		},
	}
	if mutate != nil {
		mutate(base)
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseEmailChangedTrimsSpaces(t *testing.T) {
	raw := emailChangedRaw(t, func(m map[string]any) {
		d := m["data"].(map[string]any)
		d["newEmail"] = "  next@example.com  "
		d["oldEmail"] = "  prev@example.com  "
		d["userId"] = "  " + d["userId"].(string) + "  "
	})
	evt, err := ParseEmailChanged(raw)
	if err != nil {
		t.Fatalf("con espacios debería aceptar tras trim: %v", err)
	}
	if evt.Data.NewEmail != "next@example.com" || evt.Data.OldEmail != "prev@example.com" {
		t.Fatalf("trim no aplicado: %+v", evt.Data)
	}
	if strings.TrimSpace(evt.Data.UserID) != evt.Data.UserID {
		t.Fatalf("userId sin trim: %q", evt.Data.UserID)
	}
}

func TestParseEmailChangedRejectsInvalidEmails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"newEmail sin arroba", func(m map[string]any) { m["data"].(map[string]any)["newEmail"] = "no-email" }},
		{"newEmail vacío", func(m map[string]any) { m["data"].(map[string]any)["newEmail"] = "" }},
		{"newEmail solo espacios", func(m map[string]any) { m["data"].(map[string]any)["newEmail"] = "   " }},
		{"oldEmail inválido", func(m map[string]any) { m["data"].(map[string]any)["oldEmail"] = "no-email" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseEmailChanged(emailChangedRaw(t, tc.mutate)); err == nil {
				t.Fatalf("%s debería rechazarse", tc.name)
			}
		})
	}
}

func TestParseEmailChangedRejectsInvalidUUIDs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"userId no uuid", func(m map[string]any) { m["data"].(map[string]any)["userId"] = "no-uuid" }},
		{"eventId no uuid", func(m map[string]any) { m["metadata"].(map[string]any)["eventId"] = "xxx" }},
		{"correlationId no uuid", func(m map[string]any) { m["metadata"].(map[string]any)["correlationId"] = "xxx" }},
		{"sin metadata", func(m map[string]any) { delete(m, "metadata") }},
		{"sin data", func(m map[string]any) { delete(m, "data") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseEmailChanged(emailChangedRaw(t, tc.mutate)); err == nil {
				t.Fatalf("%s debería rechazarse", tc.name)
			}
		})
	}
}

func TestParseEmailChangedAcceptsOffsetDatetime(t *testing.T) {
	raw := emailChangedRaw(t, func(m map[string]any) {
		m["metadata"].(map[string]any)["occurredAt"] = "2026-10-26T10:00:00-05:00"
	})
	if _, err := ParseEmailChanged(raw); err != nil {
		t.Fatalf("RFC3339 con offset debería aceptar: %v", err)
	}
}

func TestParseEmailChangedIgnoresUnknownFields(t *testing.T) {
	raw := emailChangedRaw(t, func(m map[string]any) {
		m["data"].(map[string]any)["extra"] = "ignorar"
		m["metadata"].(map[string]any)["extra"] = "ignorar"
	})
	if _, err := ParseEmailChanged(raw); err != nil {
		t.Fatalf("campos extra deberían ignorarse: %v", err)
	}
}
