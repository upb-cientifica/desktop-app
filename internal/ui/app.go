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
	// miUnidad es la vista de Mi unidad, donde el botón «Nuevo» sube y crea.
	miUnidad *vistaDeArchivos
	// abrirEnMiUnidad lleva a Mi unidad y entra en una carpeta (lo usan los
	// trabajos MPI para abrir sus resultados).
	abrirEnMiUnidad func(ruta string)

	mu               sync.Mutex
	sincroCli        *sincro.Cliente
	programador      *sincro.Programador
	vigia            *sincro.Vigia
	vigiaCarpeta     string
	generacionSincro int
	vistaSincro      *vistaSincronizacion
	contenidoSincro  fyne.CanvasObject
}

func Nueva(cfg config.Config, cli *bus.Cliente, ses *sesion.Sesion) *App {
	a := &App{fyne: app.NewWithID("co.edu.upb.cientifica.escritorio"), cfg: cfg, cli: cli, ses: ses}
	a.aplicarTema(a.preferenciaDeTema())
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

	a.win.SetOnClosed(a.detenerSincronizacion)
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

// preferenciaDeTema lee lo que eligió el usuario; por defecto, seguir al
// sistema operativo.
func (a *App) preferenciaDeTema() Preferencia {
	switch p := Preferencia(a.fyne.Preferences().StringWithFallback("tema", string(TemaSistema))); p {
	case TemaClaro, TemaOscuro:
		return p
	default:
		return TemaSistema
	}
}

// aplicarTema cambia el tema en caliente y lo recuerda para la próxima vez.
func (a *App) aplicarTema(p Preferencia) {
	a.fyne.Preferences().SetString("tema", string(p))
	a.fyne.Settings().SetTheme(temaUPB{preferencia: p})
}

func defaultCarpetaSincro() string { return config.CarpetaSincroPorDefecto() }

// aLaPapelera manda a la papelera el archivo del Home del que salió una foto
// o un video. Si no se sabe de cuál salió —publicaciones viejas, o subidas que
// no pasaron por el Home— no hay nada que mover.
func (a *App) aLaPapelera(ctx context.Context, rutaHome string) error {
	if rutaHome == "" {
		return nil
	}
	return a.cli.Eliminar(ctx, rutaHome, false)
}

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
	if a.vigia != nil && (p.Modo != sincro.Automatico || a.vigiaCarpeta != p.Carpeta) {
		a.vigia.Detener()
		a.vigia = nil
		a.generacionSincro++
		if a.vistaSincro != nil {
			a.vistaSincro.pendiente = false
			a.vistaSincro.pendienteSubir = false
			if a.vistaSincro.cancelar != nil {
				a.vistaSincro.cancelar()
			}
		}
	}
	if p.Modo == sincro.Automatico && a.vigia == nil && a.ses.Abierta() {
		generacion := a.generacionSincro
		v, err := sincro.NuevoVigia(p.Carpeta, a.cfg.SincroAddr, nil, func() {
			fyne.Do(func() {
				if generacion == a.generacionSincro && a.ses.Abierta() && a.vigia != nil && a.vigia.EnRed() {
					a.pasadaProgramada()
				}
			})
		}, func(dentro bool) {
			fyne.Do(func() {
				if generacion == a.generacionSincro && a.vistaSincro != nil {
					a.vistaSincro.mostrarRed(dentro)
				}
			})
		})
		if err != nil {
			a.error(err)
			return
		}
		a.vigia, a.vigiaCarpeta = v, p.Carpeta
	}
	if a.vistaSincro != nil {
		a.vistaSincro.mostrarRed(a.vigia != nil && a.vigia.EnRed())
	}
}

// marcarSincronizacion recuerda que acaba de correr una pasada, para que el
// horario no la repita.
func (a *App) marcarSincronizacion() {
	p := a.arrancarProgramador().MarcarEjecucion(time.Now())
	if a.vistaSincro != nil {
		a.vistaSincro.programa.UltimaEjecucion = p.UltimaEjecucion
	}
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

// Las pasadas usan la misma cola y la misma bitácora que el botón manual.
func (a *App) sincronizacionProgramada() {
	fyne.Do(func() {
		if a.programador != nil && a.programador.Toca(time.Now()) {
			a.pasadaProgramada()
		}
	})
}

func (a *App) pasadaProgramada() {
	if !a.ses.Abierta() {
		return
	}
	a.vistaSincronizacion()
	if !sincro.CargarEstado(a.vistaSincro.programa.Carpeta).Registrado() {
		return
	}
	if a.vistaSincro.corriendo {
		if !a.vistaSincro.pendiente {
			a.vistaSincro.pendienteAutomatico = a.vistaSincro.programa.Modo == sincro.Automatico
		}
		a.vistaSincro.pendiente = true
		a.vistaSincro.pendienteSubir = true
		return
	}
	a.vistaSincro.sincronizar(true)
}

func (a *App) detenerSincronizacion() {
	a.generacionSincro++
	if a.vigia != nil {
		a.vigia.Detener()
		a.vigia = nil
	}
	if a.programador != nil {
		a.programador.Detener()
	}
	if a.vistaSincro != nil {
		a.vistaSincro.pendiente = false
		if a.vistaSincro.cancelar != nil {
			a.vistaSincro.cancelar()
		}
	}
	// La vista es de la cuenta que cerró sesión: la siguiente se arma de nuevo.
	a.vistaSincro, a.contenidoSincro = nil, nil
}
