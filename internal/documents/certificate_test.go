package documents

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func TestCertificateValidateOK(t *testing.T) {
	certID, _ := uuid.NewV7()
	userID, _ := uuid.NewV7()
	c := Certificate{
		ID:                   certID.String(),
		UserID:               userID.String(),
		AffiliationStartDate: time.Now().UTC(),
		IssuedAt:             time.Now().UTC(),
		FileKey:              "certificates/" + userID.String() + "/" + certID.String() + ".pdf",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("válido rechazado: %v", err)
	}
}

func TestCertificateValidateTable(t *testing.T) {
	certID, _ := uuid.NewV7()
	userID, _ := uuid.NewV7()
	now := time.Now().UTC()
	valid := Certificate{
		ID:                   certID.String(),
		UserID:               userID.String(),
		AffiliationStartDate: now,
		IssuedAt:             now,
		FileKey:              "certificates/x/y.pdf",
	}
	cases := []struct {
		name   string
		mutate func(*Certificate)
	}{
		{"id vacío", func(c *Certificate) { c.ID = "" }},
		{"id inválido", func(c *Certificate) { c.ID = "no-uuid" }},
		{"user vacío", func(c *Certificate) { c.UserID = "  " }},
		{"user inválido", func(c *Certificate) { c.UserID = "xxx" }},
		{"affiliation zero", func(c *Certificate) { c.AffiliationStartDate = time.Time{} }},
		{"issued zero", func(c *Certificate) { c.IssuedAt = time.Time{} }},
		{"filekey vacío", func(c *Certificate) { c.FileKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("debería fallar")
			}
		})
	}
}

func TestCertificateEnsureUTC(t *testing.T) {
	certID, _ := uuid.NewV7()
	userID, _ := uuid.NewV7()
	c := Certificate{
		ID:                   certID.String(),
		UserID:               userID.String(),
		AffiliationStartDate: time.Now(),
		IssuedAt:             time.Now(),
		FileKey:              "k",
	}
	c.EnsureUTC()
	if c.IssuedAt.Location() != time.UTC || c.AffiliationStartDate.Location() != time.UTC {
		t.Fatal("debería normalizar a UTC")
	}
}
