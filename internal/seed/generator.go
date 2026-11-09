package seed

import (
	"crypto/sha256"
	"fmt"
	"time"

	// Asegura America/Bogota en contenedores sin zoneinfo.
	_ "time/tzdata"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

// AppointmentsPerType es el número de citas por tipo y usuario.
// Por defecto 2 por tipo (6 por usuario, 18 en total).
const AppointmentsPerType = 2

// durations es la duración en minutos por tipo de cita.
var durations = map[documents.AppointmentType]int{
	documents.AppointmentTypeMedicinaGeneral: 30,
	documents.AppointmentTypeOdontologia:     45,
	documents.AppointmentTypePsicologia:      60,
}

// seedPhysicians son 1-2 médicos ficticios por tipo (nombres inventados).
var seedPhysicians = map[documents.AppointmentType][]string{
	documents.AppointmentTypeMedicinaGeneral: {"Dra. Laura Cifuentes", "Dr. Andrés Molina"},
	documents.AppointmentTypeOdontologia:     {"Dra. Sofía Herrera", "Dr. Carlos Quintero"},
	documents.AppointmentTypePsicologia:      {"Dra. Paula Restrepo", "Dr. Felipe Vargas"},
}

// seedCenters son dos sedes ficticias en Bogotá (nombres y direcciones inventados).
var seedCenters = []documents.MedicalCenter{
	{MedicalCenterID: centerID("Sede Norte"), Name: "Sede Norte", Location: "Calle 127 # 45-10, Bogota"},
	{MedicalCenterID: centerID("Sede Occidente"), Name: "Sede Occidente", Location: "Av. Esperanza # 60-20, Bogota"},
}

// seedNotes son notas genéricas en español por tipo (sin diagnósticos graves).
var seedNotes = map[documents.AppointmentType][]string{
	documents.AppointmentTypeMedicinaGeneral: {
		"Control general sin hallazgos relevantes. Se refuerzan hábitos saludables y control en 12 meses.",
		"Tensión arterial estable. Se mantiene tratamiento actual y se ordena chequeo de laboratorio de rutina.",
		"Cuadro gripal leve en resolución. Reposo relativo e hidratación por 3 días.",
	},
	documents.AppointmentTypeOdontologia: {
		"Limpieza dental y aplicación de flúor. Encías sanas, sin caries nuevas.",
		"Revisión dental de control sin hallazgos. Se refuerza técnica de cepillado e hilo dental.",
		"Obturación preventiva en molar sin complicaciones. Control en 6 meses.",
	},
	documents.AppointmentTypePsicologia: {
		"Sesión de seguimiento: la usuaria reporta mejoría en el sueño. Se acuerdan estrategias de manejo del estrés.",
		"Sesión de seguimiento: se revisan avances en hábitos de autocuidado. Próximo control en un mes.",
		"Primera sesión de orientación: se establece plan de acompañamiento y señales de alarma.",
	},
}

// centerID genera el id estable de una sede a partir de su nombre.
func centerID(name string) string {
	return "SEDE-" + uuid.NewV5(uuid.NamespaceOID, "kenko:center:"+name).String()[:8]
}

// physicianID genera el id estable de un médico a partir de su nombre.
func physicianID(name string) string {
	return "MED-" + uuid.NewV5(uuid.NamespaceOID, "kenko:physician:"+name).String()[:8]
}

// GenerateAppointments crea 2 citas por tipo para el usuario en los últimos
// 12 meses (lunes a viernes, ninguna futura, sin repetir día y hora).
// Los ids son UUID v5 de (userId, tipo, índice): re-ejecutar actualiza.
func GenerateAppointments(u DemoUser, now time.Time) []documents.CompletedAppointment {
	now = now.UTC()
	out := make([]documents.CompletedAppointment, 0, len(documents.AppointmentTypes)*AppointmentsPerType)
	used := map[string]bool{}
	for _, typ := range documents.AppointmentTypes {
		for i := 0; i < AppointmentsPerType; i++ {
			a := buildAppointment(u, typ, i, now, used)
			out = append(out, a)
			used[a.Date+"|"+a.StartTime] = true
		}
	}
	return out
}

// buildAppointment genera una cita determinística del tipo e índice dados.
func buildAppointment(u DemoUser, typ documents.AppointmentType, idx int, now time.Time, used map[string]bool) documents.CompletedAppointment {
	salt := string(typ) + ":" + fmt.Sprint(idx)
	// Días atrás entre 1 y 365 (nunca hoy ni futuro).
	daysAgo := 1 + int(hashByte(u.UserID, "days-"+salt))%365
	// Hora de inicio entre 08:00 y 16:00 en pasos de 15 min (fin <= 17:00).
	startMin := 8*60 + int(hashByte(u.UserID, "time-"+salt))%(8*60)
	startMin = (startMin / 15) * 15
	// Ajusta para que el fin no pase las 17:00.
	if end := startMin + durations[typ]; end > 17*60 {
		startMin = 17*60 - durations[typ]
	}
	// Busca día y hora libres (lun-vie, pasado, sin repetir).
	var day time.Time
	var start string
	for tries := 0; tries < 800; tries++ {
		candidate := now.AddDate(0, 0, -(daysAgo + tries))
		if wd := bogotaWeekday(candidate); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		candidateStart := shiftMinutes(startMin, tries*15)
		if end := candidateStart + durations[typ]; candidateStart < 8*60 || end > 17*60 {
			continue
		}
		dateStr := candidate.Format("2006-01-02")
		startStr := fmt.Sprintf("%02d:%02d", candidateStart/60, candidateStart%60)
		if used[dateStr+"|"+startStr] {
			continue
		}
		// completedAt no puede ser futuro respecto a now.
		if completed := appointmentEndUTC(dateStr, startStr, durations[typ]); completed.After(now) {
			daysAgo++
			continue
		}
		day = candidate
		start = startStr
		daysAgo += tries
		break
	}
	// Respaldo determinístico (no debería alcanzarse).
	if start == "" {
		day = prevWeekday(now.AddDate(0, 0, -daysAgo))
		start = fmt.Sprintf("%02d:%02d", startMin/60, startMin%60)
	}
	dateStr := day.Format("2006-01-02")
	endStr := addMinutes(start, durations[typ])
	physicians := seedPhysicians[typ]
	physName := physicians[int(hashByte(u.UserID, "phy-"+salt))%len(physicians)]
	center := seedCenters[int(hashByte(u.UserID, "cen-"+salt))%len(seedCenters)]
	notes := seedNotes[typ]
	return documents.CompletedAppointment{
		ID:        uuid.NewV5(uuid.NamespaceOID, "kenko:appointment:"+u.UserID+":"+string(typ)+":"+fmt.Sprint(idx)).String(),
		UserID:    u.UserID,
		Type:      typ,
		Date:      dateStr,
		StartTime: start,
		EndTime:   endStr,
		Physician: documents.Physician{
			PhysicianID: physicianID(physName),
			Name:        physName,
		},
		MedicalCenter: center,
		Status:        documents.AppointmentStatusCompleted,
		Notes:         notes[int(hashByte(u.UserID, "note-"+salt))%len(notes)],
		CompletedAt:   appointmentEndUTC(dateStr, endStr, 0),
	}
}

// appointmentEndUTC interpreta fecha + hora como hora de Colombia y la
// convierte a UTC. shiftMin desplaza minutos (0 en uso normal).
func appointmentEndUTC(dateStr, hm string, shiftMin int) time.Time {
	loc, err := time.LoadLocation("America/Bogota")
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", dateStr+" "+hm, loc)
	if err != nil {
		return time.Time{}
	}
	return t.Add(time.Duration(shiftMin) * time.Minute).UTC()
}

// bogotaWeekday devuelve el día de la semana en hora de Colombia.
func bogotaWeekday(t time.Time) time.Weekday {
	loc, err := time.LoadLocation("America/Bogota")
	if err != nil {
		return t.Weekday()
	}
	return t.In(loc).Weekday()
}

// prevWeekday retrocede hasta lunes-viernes.
func prevWeekday(t time.Time) time.Time {
	for {
		if wd := bogotaWeekday(t); wd != time.Saturday && wd != time.Sunday {
			return t
		}
		t = t.AddDate(0, 0, -1)
	}
}

// shiftMinutes desplaza minutos manteniendo el rango del día (0-1439).
func shiftMinutes(base, delta int) int {
	v := (base + delta) % (24 * 60)
	if v < 0 {
		v += 24 * 60
	}
	return v
}

// addMinutes suma minutos a una hora "HH:MM".
func addMinutes(hm string, minutes int) string {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		return hm
	}
	return t.Add(time.Duration(minutes) * time.Minute).Format("15:04")
}

// hashByte retorna un byte determinista del SHA-256 de (userId, salt).
func hashByte(userID, salt string) byte {
	sum := sha256.Sum256([]byte(userID + ":" + salt))
	return sum[0]
}
