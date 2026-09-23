// Package ui arma la ventana de la aplicación.
//
// Regla de oro de Fyne: los cambios en la interfaz se hacen en el hilo de la
// interfaz. Cualquier llamada al bus se lanza en una gorutina para no congelar
// la ventana, y lo que devuelve se pinta dentro de fyne.Do.
package ui

import (
	"context"
	"net/url"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/config"
	"github.com/upb-cientifica/desktop-app/internal/sesion"
	"github.com/upb-cientifica/desktop-app/internal/sincro"
)

type App struct {
	fyne fyne.App
	win  fyne.Window
	cfg  config.Config
	cli  *bus.Cliente
	ses  *sesion.Sesion

	// refrescarCuota lo instala el marco principal: las vistas lo llaman
	// cuando cambian el contenido del Home.
	refrescarCuota func()

	mu          sync.Mutex
	sincroCli   *sincro.Cliente
	programador *sincro.Programador
}

func Nueva(cfg config.Config, cli *bus.Cliente, ses *sesion.Sesion) *App {
	a := &App{fyne: app.NewWithID("co.edu.upb.cientifica.escritorio"), cfg: cfg, cli: cli, ses: ses}
	a.win = a.fyne.NewWindow("UPB-CIENTÍFICA")
	a.win.Resize(fyne.NewSize(1100, 720))
	a.win.CenterOnScreen()
	return a
}

// Ejecutar muestra la ventana y no vuelve hasta que se cierra.
func (a *App) Ejecutar() {
	// Si la sesión guardada se cae mientras se trabaja, se vuelve a la
	// pantalla de entrada en vez de dejar botones que fallan uno a uno.
	a.ses.AlCerrarse = func() {
		fyne.Do(func() {
			a.mostrarEntrada()
			dialog.ShowInformation("Sesión terminada", "La sesión venció. Entra de nuevo.", a.win)
		})
	}

	// Cuando el perfil termina de llegar, el marco se redibuja con el nombre
	// y, si toca, con la sección de Administración.
	a.ses.AlActualizarse = func() {
		fyne.Do(func() {
			if a.ses.Abierta() {
				a.mostrarPrincipal()
			}
		})
	}

	a.mostrarCargando("Reanudando la sesión…")
	go func() {
		ctx, cancelar := bus.ConPlazo(context.Background())
		defer cancelar()
		ok := a.ses.Reanudar(ctx)
		fyne.Do(func() {
			if ok {
				a.mostrarPrincipal()
			} else {
				a.mostrarEntrada()
			}
		})
	}()

	a.win.ShowAndRun()
}

// enSegundoPlano corre el trabajo fuera del hilo de la interfaz y entrega el
// resultado dentro de ella.
func enSegundoPlano[T any](trabajo func(context.Context) (T, error), alTerminar func(T, error)) {
	go func() {
		ctx, cancelar := bus.ConPlazo(context.Background())
		defer cancelar()
		v, err := trabajo(ctx)
		fyne.Do(func() { alTerminar(v, err) })
	}()
}

func (a *App) error(err error) {
	dialog.ShowError(err, a.win)
}

// sincro abre —una sola vez— la conexión gRPC con File Sync. El token se
// consulta en cada llamada, porque se renueva cada quince minutos.
func (a *App) sincro() (*sincro.Cliente, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sincroCli != nil {
		return a.sincroCli, nil
	}
	cli, err := sincro.Conectar(a.cfg.SincroAddr, a.cli.Token)
	if err != nil {
		return nil, err
	}
	a.sincroCli = cli
	return cli, nil
}

func defaultCarpetaSincro() string { return config.CarpetaSincroPorDefecto() }

func legible(bytes int64) string { return bus.Legible(bytes) }

// abrirEnElSistema deja que el explorador de archivos del sistema muestre la
// carpeta sincronizada.
func (a *App) abrirEnElSistema(ruta string) {
	u, err := url.Parse("file://" + ruta)
	if err != nil {
		a.error(err)
		return
	}
	if err := a.fyne.OpenURL(u); err != nil {
		a.error(err)
	}
}

// guardarPrograma deja en disco cuándo hay que sincronizar y se lo pasa al
// programador, que es quien mira el reloj.
func (a *App) guardarPrograma(p sincro.Programa) {
	if err := sincro.GuardarPrograma(a.cfg.DirDatos, p); err != nil {
		a.error(err)
		return
	}
	a.arrancarProgramador().Aplicar(p)
}

// marcarSincronizacion recuerda que acaba de correr una pasada, para que el
// horario no la repita.
func (a *App) marcarSincronizacion() {
	p := a.arrancarProgramador().MarcarEjecucion(time.Now())
	_ = sincro.GuardarPrograma(a.cfg.DirDatos, p)
}

func (a *App) arrancarProgramador() *sincro.Programador {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.programador == nil {
		a.programador = sincro.NuevoProgramador(a.sincronizacionProgramada)
	}
	return a.programador
}

// sincronizacionProgramada es la pasada que dispara el horario, sin que nadie
// esté mirando la ventana. Lo que ocurra queda en el estado de la carpeta y se
// ve al abrir la sección.
func (a *App) sincronizacionProgramada() {
	if !a.ses.Abierta() {
		return
	}
	p := sincro.CargarPrograma(a.cfg.DirDatos, defaultCarpetaSincro())
	st := sincro.CargarEstado(p.Carpeta)
	if !st.Registrado() {
		return
	}
	cli, err := a.sincro()
	if err != nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), time.Hour)
	defer cancelar()
	cli.Sincronizar(ctx, st, p.Carpeta, sincro.Opciones{Subir: true, Borrar: p.PropagarBorrados})
	_ = st.Guardar()
	a.marcarSincronizacion()
}
