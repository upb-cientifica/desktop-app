package sincro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// El enunciado pide que la sincronización "pueda programarse de manera tal que
// en un horario establecido se realice". Aquí están las dos formas de hacerlo:
// cada cierto rato, o todos los días a una hora. El programa se guarda junto a
// los datos de la aplicación, no dentro de la carpeta sincronizada, porque es
// una preferencia de este equipo.

type Modo string

const (
	Manual    Modo = "manual"
	Intervalo Modo = "intervalo"
	Diario    Modo = "diario"
)

type Programa struct {
	Carpeta          string `json:"carpeta"`
	Modo             Modo   `json:"modo"`
	IntervaloMin     int    `json:"intervaloMin"` // para Intervalo
	Hora             string `json:"hora"`         // para Diario, "HH:MM"
	PropagarBorrados bool   `json:"propagarBorrados"`
	UltimaEjecucion  string `json:"ultimaEjecucion"` // RFC3339
}

func ProgramaPorDefecto(carpeta string) Programa {
	return Programa{Carpeta: carpeta, Modo: Manual, IntervaloMin: 15, Hora: "20:00"}
}

func rutaPrograma(dirDatos string) string { return filepath.Join(dirDatos, "sincronizacion.json") }

func CargarPrograma(dirDatos, carpetaPorDefecto string) Programa {
	p := ProgramaPorDefecto(carpetaPorDefecto)
	b, err := os.ReadFile(rutaPrograma(dirDatos))
	if err != nil {
		return p
	}
	_ = json.Unmarshal(b, &p)
	if p.IntervaloMin <= 0 {
		p.IntervaloMin = 15
	}
	if p.Carpeta == "" {
		p.Carpeta = carpetaPorDefecto
	}
	return p
}

func GuardarPrograma(dirDatos string, p Programa) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rutaPrograma(dirDatos), b, 0o600)
}

// Programador dispara la sincronización cuando toca. Revisa cada medio minuto,
// que es suficiente para una tarea de este tipo y no gasta nada.
type Programador struct {
	mu        sync.Mutex
	programa  Programa
	ejecutar  func()
	parar     chan struct{}
	corriendo bool
}

func NuevoProgramador(ejecutar func()) *Programador {
	return &Programador{ejecutar: ejecutar, parar: make(chan struct{})}
}

func (p *Programador) Aplicar(pr Programa) {
	p.mu.Lock()
	p.programa = pr
	p.mu.Unlock()
	p.arrancar()
}

func (p *Programador) arrancar() {
	p.mu.Lock()
	if p.corriendo {
		p.mu.Unlock()
		return
	}
	p.corriendo = true
	p.mu.Unlock()

	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.parar:
				return
			case ahora := <-t.C:
				if p.Toca(ahora) {
					p.ejecutar()
				}
			}
		}
	}()
}

func (p *Programador) Detener() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.corriendo {
		close(p.parar)
		p.parar = make(chan struct{})
		p.corriendo = false
	}
}

// MarcarEjecucion recuerda cuándo corrió la última pasada.
func (p *Programador) MarcarEjecucion(cuando time.Time) Programa {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.programa.UltimaEjecucion = cuando.Format(time.RFC3339)
	return p.programa
}

// Toca decide si en este instante corresponde sincronizar.
func (p *Programador) Toca(ahora time.Time) bool {
	p.mu.Lock()
	pr := p.programa
	p.mu.Unlock()
	return Toca(pr, ahora)
}

// Toca es la regla, aparte del reloj, para poder probarla.
func Toca(pr Programa, ahora time.Time) bool {
	ultima := ultimaEjecucion(pr)
	switch pr.Modo {
	case Intervalo:
		if ultima.IsZero() {
			return true
		}
		return ahora.Sub(ultima) >= time.Duration(pr.IntervaloMin)*time.Minute
	case Diario:
		h, m, ok := horaDe(pr.Hora)
		if !ok {
			return false
		}
		programada := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), h, m, 0, 0, ahora.Location())
		if ahora.Before(programada) {
			return false
		}
		// Ya pasó la hora de hoy: corre si aún no se ha ejecutado después.
		return ultima.Before(programada)
	default:
		return false
	}
}

func ultimaEjecucion(pr Programa) time.Time {
	if pr.UltimaEjecucion == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, pr.UltimaEjecucion)
	if err != nil {
		return time.Time{}
	}
	return t
}

func horaDe(hhmm string) (int, int, bool) {
	partes := strings.Split(strings.TrimSpace(hhmm), ":")
	if len(partes) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(partes[0])
	m, err2 := strconv.Atoi(partes[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// Descripcion explica en una línea qué va a pasar, para mostrarlo en la ventana.
func (pr Programa) Descripcion() string {
	switch pr.Modo {
	case Intervalo:
		return "Cada " + strconv.Itoa(pr.IntervaloMin) + " minutos"
	case Diario:
		return "Todos los días a las " + pr.Hora
	default:
		return "Solo cuando pulses Sincronizar"
	}
}
