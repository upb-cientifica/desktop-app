// Package ui arma la ventana de la aplicación.
//
// Regla de oro de Fyne: los cambios en la interfaz se hacen en el hilo de la
// interfaz. Cualquier llamada al bus se lanza en una gorutina para no congelar
// la ventana, y lo que devuelve se pinta dentro de fyne.Do.
package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/config"
	"github.com/upb-cientifica/desktop-app/internal/sesion"
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
