package seed

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

// Listas cerradas de datos ficticios (sin instituciones reales).
var seedPhysicians = []string{
	"Dra. Laura Cifuentes",
	"Dr. Andrés Molina",
	"Dra. Sofía Herrera",
	"Dr. Carlos Quintero",
	"Dra. Paula Restrepo",
}

var seedCenters = []documents.MedicalCenter{
	{MedicalCenterID: "SEDE-NORTE", Name: "Sede Norte", Location: "Calle 127 # 45-10, Bogota"},
	{MedicalCenterID: "SEDE-SALITRE", Name: "Sede Salitre", Location: "Av. Esperanza # 60-20, Bogota"},
	{MedicalCenterID: "SEDE-SUR", Name: "Sede Sur", Location: "Av. Caracas # 50-30, Bogota"},
}

var seedStartTimes = []string{"08:00", "09:30", "11:00", "14:00", "16:30"}

// seedNotes da 2-3 diagnósticos verosímiles por especialidad.
var seedNotes = map[string][]string{
	"MEDICINA_GENERAL": {
		"Control de tensión arterial: estable, continuar tratamiento.",
		"Chequeo general anual sin hallazgos relevantes.",
		"Cuadro gripal leve: reposo e hidratación por 3 días.",
	},
	"ODONTOLOGIA": {
		"Limpieza dental y aplicación de flúor, sin caries.",
		"Obturación en molar superior derecho.",
		"Revisión de ortodoncia: ajuste de brackets.",
	},
	"OPTOMETRIA": {
		"Agudeza visual 20/25, se ordenan lentes de descanso.",
		"Control de miopía estable respecto al año anterior.",
		"Ojo seco leve: lubricante ocular cada 8 horas.",
	},
	"DERMATOLOGIA": {
		"Dermatitis leve en antebrazo: crema hidratante medicada.",
		"Revisión de lunares sin cambios sospechosos.",
		"Acné moderado: tratamiento tópico por 60 días.",
	},
	"CARDIOLOGIA": {
		"Electrocardiograma normal, control en 12 meses.",
		"Colesterol ligeramente alto: ajuste de dieta.",
		"Prueba de esfuerzo sin alteraciones.",
	},
}

// AppointmentCount retorna 4, 5 o 6 citas según el hash del userId.
func AppointmentCount(userID string) int {
	return 4 + int(hashByte(userID, "count")%3)
}

// GenerateAppointments crea las citas del usuario entre hoy y hace 12 meses.
// Los ids son UUID v5 de (userId, índice): re-ejecutar actualiza, no duplica.
func GenerateAppointments(u DemoUser, now time.Time) []documents.CompletedAppointment {
	now = now.UTC()
	n := AppointmentCount(u.UserID)
	out := make([]documents.CompletedAppointment, 0, n)
	for i := 0; i < n; i++ {
		// 25..40 días la más reciente; ~60 días entre citas (máx. ~339 días).
		daysAgo := 25 + i*60 + int(hashByte(u.UserID, fmt.Sprintf("days-%d", i))%15)
		day := now.AddDate(0, 0, -daysAgo)
		typ := documents.AppointmentTypes[int(hashByte(u.UserID, fmt.Sprintf("type-%d", i)))%len(documents.AppointmentTypes)]
		physician := seedPhysicians[int(hashByte(u.UserID, fmt.Sprintf("phy-%d", i)))%len(seedPhysicians)]
		center := seedCenters[int(hashByte(u.UserID, fmt.Sprintf("cen-%d", i)))%len(seedCenters)]
		start := seedStartTimes[int(hashByte(u.UserID, fmt.Sprintf("time-%d", i)))%len(seedStartTimes)]
		notes := seedNotes[typ]
		out = append(out, documents.CompletedAppointment{
			ID:            uuid.NewV5(uuid.NamespaceOID, "kenko:appointment:"+u.UserID+":"+fmt.Sprint(i)).String(),
			UserID:        u.UserID,
			Type:          typ,
			Date:          day.Format("2006-01-02"),
			StartTime:     start,
			EndTime:       endTime(start),
			Physician:     documents.Physician{PhysicianID: fmt.Sprintf("MED-%d", int(hashByte(u.UserID, fmt.Sprintf("phyid-%d", i)))%900+100), Name: physician},
			MedicalCenter: center,
			Status:        documents.AppointmentStatusCompleted,
			Notes:         notes[int(hashByte(u.UserID, fmt.Sprintf("note-%d", i)))%len(notes)],
			CompletedAt:   time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, time.UTC),
		})
	}
	return out
}

// endTime suma 30 minutos a la hora de inicio (texto HH:MM).
func endTime(start string) string {
	t, err := time.Parse("15:04", start)
	if err != nil {
		return start
	}
	return t.Add(30 * time.Minute).Format("15:04")
}

// hashByte retorna un byte determinista del SHA-256 de (userId, salt).
func hashByte(userID, salt string) byte {
	sum := sha256.Sum256([]byte(userID + ":" + salt))
	return sum[0]
}
