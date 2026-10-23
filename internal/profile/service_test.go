package profile

import (
	"errors"
	"testing"
)

func TestValidatePatchOK(t *testing.T) {
	raw := map[string]any{
		"phone":   "+573009998877",
		"address": map[string]any{"line": "Calle 10 # 5-20", "city": "Medellin"},
	}
	in, err := ValidatePatch(raw)
	if err != nil {
		t.Fatalf("patch válido rechazado: %v", err)
	}
	if in.Phone == nil || !in.HasAddress {
		t.Fatal("campos no mapeados")
	}
}

func TestValidatePatchRejectsEmailAndPassword(t *testing.T) {
	for _, raw := range []map[string]any{
		{"email": "x@y.com"},
		{"password": "secret"},
		{"phone": "+57300", "email": "x@y.com"},
		{"first_name": "Otro"},
		{"address": map[string]any{"line": "solo linea"}},
		{"address": map[string]any{"line": "L", "city": "C", "extra": "x"}},
		{},
	} {
		if _, err := ValidatePatch(raw); !errors.Is(err, ErrInvalidPatch) {
			t.Fatalf("debería rechazar %v", raw)
		}
	}
}
