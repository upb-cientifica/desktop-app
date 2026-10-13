package sincro

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Marcador permite comprobar la red sin depender de un servidor en las pruebas.
type Marcador func(context.Context, string, string) (net.Conn, error)

type Vigia struct {
	enRed     atomic.Bool
	cancelar  context.CancelFunc
	terminado chan struct{}
}

func (v *Vigia) EnRed() bool { return v.enRed.Load() }
func (v *Vigia) Detener()    { v.cancelar(); <-v.terminado }

func NuevoVigia(carpeta, direccion string, marcar Marcador, ejecutar func(), cambio func(bool)) (*Vigia, error) {
	return nuevoVigia(carpeta, direccion, marcar, ejecutar, cambio, 20*time.Second, 2*time.Minute, 3*time.Second)
}

func nuevoVigia(carpeta, direccion string, marcar Marcador, ejecutar func(), cambio func(bool), sondeo, remoto, rebote time.Duration) (*Vigia, error) {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	agregar := func(raiz string) error {
		return filepath.WalkDir(raiz, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if ignorarArchivo(d.Name()) || d.Name() == ".git" || d.Name() == ".filesync" {
					return filepath.SkipDir
				}
				return w.Add(p)
			}
			return nil
		})
	}
	if err := agregar(carpeta); err != nil {
		w.Close()
		return nil, err
	}
	if marcar == nil {
		marcar = (&net.Dialer{}).DialContext
	}
	ctx, cancelar := context.WithCancel(context.Background())
	v := &Vigia{cancelar: cancelar, terminado: make(chan struct{})}
	go func() {
		defer close(v.terminado)
		defer w.Close()
		red := time.NewTicker(sondeo)
		defer red.Stop()
		traer := time.NewTicker(remoto)
		defer traer.Stop()
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		var cambios <-chan time.Time
		terminado := make(chan struct{}, 1)
		ocupado, pendiente := false, false
		disparar := func() {
			if !v.EnRed() || ctx.Err() != nil {
				return
			}
			if ocupado {
				pendiente = true
				return
			}
			ocupado = true
			go func() { ejecutar(); terminado <- struct{}{} }()
		}
		comprobar := func() {
			plazo, fin := context.WithTimeout(ctx, 2*time.Second)
			conn, err := marcar(plazo, "tcp", direccion)
			fin()
			if conn != nil {
				conn.Close()
			}
			if ctx.Err() != nil {
				return
			}
			dentro := err == nil
			anterior := v.enRed.Swap(dentro)
			if anterior != dentro {
				if cambio != nil {
					cambio(dentro)
				}
				if dentro {
					disparar()
				}
			}
		}
		comprobar()
		for {
			select {
			case <-ctx.Done():
				return
			case <-red.C:
				comprobar()
			case <-traer.C:
				disparar()
			case <-terminado:
				ocupado = false
				if pendiente {
					pendiente = false
					disparar()
				}
			case evento, ok := <-w.Events:
				if !ok {
					return
				}
				if ignorarArchivo(filepath.Base(evento.Name)) {
					continue
				}
				if evento.Has(fsnotify.Create) {
					_ = agregar(evento.Name)
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(rebote)
				cambios = timer.C
			case <-cambios:
				cambios = nil
				disparar()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// Ante un desbordamiento, la próxima pasada vuelve a recorrer el árbol.
				_ = agregar(carpeta)
				disparar()
			}
		}
	}()
	return v, nil
}

func ignorarArchivo(nombre string) bool {
	return nombre == archivoEstado || nombre == ".DS_Store" || strings.HasPrefix(nombre, ".fs-") || strings.HasPrefix(nombre, "~") || strings.HasSuffix(nombre, "~") || strings.HasSuffix(nombre, ".tmp") || strings.HasSuffix(nombre, ".swp")
}
