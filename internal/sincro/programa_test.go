package sincro

import (
	"testing"
	"time"
)

func enPunto(hora, minuto int) time.Time {
	return time.Date(2026, 9, 22, hora, minuto, 0, 0, time.Local)
}

func TestManualNuncaDispara(t *testing.T) {
	pr := Programa{Modo: Manual}
	if Toca(pr, enPunto(20, 0)) {
		t.Fatal("el modo manual no debe disparar solo")
	}
}

func TestIntervaloDisparaLaPrimeraVezYLuegoAlCumplirse(t *testing.T) {
	pr := Programa{Modo: Intervalo, IntervaloMin: 15}
	if !Toca(pr, enPunto(9, 0)) {
		t.Fatal("sin ejecuciones previas debe disparar")
	}

	pr.UltimaEjecucion = enPunto(9, 0).Format(time.RFC3339)
	if Toca(pr, enPunto(9, 14)) {
		t.Fatal("antes de los 15 minutos no debe disparar")
	}
	if !Toca(pr, enPunto(9, 15)) {
		t.Fatal("a los 15 minutos debe disparar")
	}
}

func TestDiarioDisparaUnaVezPasadaLaHora(t *testing.T) {
	pr := Programa{Modo: Diario, Hora: "20:00"}

	if Toca(pr, enPunto(19, 59)) {
		t.Fatal("antes de la hora no debe disparar")
	}
	if !Toca(pr, enPunto(20, 0)) {
		t.Fatal("a la hora debe disparar")
	}

	// Ya corrió hoy a las 20:00: no debe repetir el resto del día.
	pr.UltimaEjecucion = enPunto(20, 0).Format(time.RFC3339)
	if Toca(pr, enPunto(23, 30)) {
		t.Fatal("no debe repetir el mismo día")
	}

	// Al día siguiente, pasada la hora, vuelve a tocar.
	maniana := enPunto(20, 1).Add(24 * time.Hour)
	if !Toca(pr, maniana) {
		t.Fatal("al día siguiente debe volver a disparar")
	}
}

func TestHoraInvalidaNoDispara(t *testing.T) {
	for _, hora := range []string{"", "25:00", "20", "20:61", "ocho"} {
		if Toca(Programa{Modo: Diario, Hora: hora}, enPunto(21, 0)) {
			t.Fatalf("la hora %q no es válida y no debería disparar", hora)
		}
	}
}

func TestDescripcion(t *testing.T) {
	if got := (Programa{Modo: Intervalo, IntervaloMin: 5}).Descripcion(); got != "Cada 5 minutos" {
		t.Fatalf("descripción: %q", got)
	}
	if got := (Programa{Modo: Diario, Hora: "07:30"}).Descripcion(); got != "Todos los días a las 07:30" {
		t.Fatalf("descripción: %q", got)
	}
}
