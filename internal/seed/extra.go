package seed

import (
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
)

// MakeExtraUsers genera n perfiles ficticios de relleno, determinísticos.
// No usan los correos demo: usan @ejemplo.invalid (dominio reservado que no
// existe en DNS; jamás se les envía correo). Los ids son UUID v5 estables
// para que re-ejecutar el seed actualice en lugar de duplicar.
func MakeExtraUsers(n int, now time.Time) []DemoUser {
	now = now.UTC()
	firstNames := []string{"Alex", "Sam", "Lucía", "Diego", "Paola", "Jorge", "Sara"}
	lastNames := []string{"Pardo León", "Mesa Ríos", "Cano Gil", "Vera Soto", "Luna Paz", "Ríos Cano", "Gil Vera"}
	genders := []string{"female", "male"}
	out := make([]DemoUser, 0, n)
	for i := 0; i < n; i++ {
		id := uuid.NewV5(uuid.NamespaceOID, fmt.Sprintf("kenko:seed:user:%d", i)).String()
		// createdAt escalonado en los últimos 12 meses.
		createdAt := now.AddDate(0, 0, -(30 + i*20)).UTC()
		birth := time.Date(1980+(i*7)%20, time.Month(1+(i*5)%12), 1+(i*11)%27, 0, 0, 0, 0, time.UTC)
		out = append(out, DemoUser{
			UserID:         id,
			DocumentType:   "CC",
			DocumentNumber: fmt.Sprintf("900000000%d", i),
			FirstName:      firstNames[i%len(firstNames)],
			LastName:       lastNames[i%len(lastNames)],
			BirthDate:      birth.Format("2006-01-02"),
			Gender:         genders[i%len(genders)],
			Email:          fmt.Sprintf("usuario%02d.demo@ejemplo.invalid", i+4),
			Phone:          fmt.Sprintf("+5730090000%02d", i),
			AddressLine:    fmt.Sprintf("Calle %d # 10-20 Apto %d", 100+i, 100+i),
			City:           "Bogota",
			CreatedAt:      createdAt,
		})
	}
	return out
}
