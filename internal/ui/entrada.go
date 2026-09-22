package ui

import (
	"context"
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// mostrarCargando ocupa la ventana con un aviso mientras se espera algo.
func (a *App) mostrarCargando(texto string) {
	barra := widget.NewProgressBarInfinite()
	a.win.SetContent(container.NewCenter(container.NewVBox(
		widget.NewLabel(texto),
		barra,
	)))
}

// mostrarEntrada pide cuenta y contraseña al Servicio de Usuarios.
func (a *App) mostrarEntrada() {
	cuenta := widget.NewEntry()
	cuenta.SetPlaceHolder("tu.cuenta o correo@" + bus.DominioCorreo)
	cuenta.SetText(a.ses.CorreoRecordado())

	clave := widget.NewPasswordEntry()
	clave.SetPlaceHolder("Contraseña")

	aviso := widget.NewLabel("")
	aviso.Wrapping = fyne.TextWrapWord
	aviso.Hide()

	boton := widget.NewButton("Entrar", nil)
	boton.Importance = widget.HighImportance

	entrar := func() {
		if cuenta.Text == "" || clave.Text == "" {
			aviso.SetText("Escribe tu cuenta y tu contraseña.")
			aviso.Show()
			return
		}
		aviso.Hide()
		boton.Disable()
		boton.SetText("Entrando…")

		usuario, password := cuenta.Text, clave.Text
		enSegundoPlano(func(ctx context.Context) (struct{}, error) {
			return struct{}{}, a.ses.Entrar(ctx, usuario, password)
		}, func(_ struct{}, err error) {
			boton.Enable()
			boton.SetText("Entrar")
			if err != nil {
				aviso.SetText(mensajeDeEntrada(err))
				aviso.Show()
				clave.SetText("")
				return
			}
			a.mostrarPrincipal()
		})
	}

	boton.OnTapped = entrar
	clave.OnSubmitted = func(string) { entrar() }
	cuenta.OnSubmitted = func(string) { a.win.Canvas().Focus(clave) }

	titulo := widget.NewLabelWithStyle("UPB-CIENTÍFICA", fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true})
	subtitulo := widget.NewLabelWithStyle("Aplicación de escritorio", fyne.TextAlignCenter,
		fyne.TextStyle{Italic: true})

	formulario := container.NewVBox(
		titulo, subtitulo,
		layout.NewSpacer(),
		widget.NewLabel("Cuenta"), cuenta,
		widget.NewLabel("Contraseña"), clave,
		aviso,
		boton,
	)

	// Una franja centrada de ancho fijo, para que el formulario no se estire
	// de lado a lado en una ventana grande.
	a.win.SetContent(container.NewCenter(container.NewGridWrap(fyne.NewSize(360, 420), formulario)))
	a.win.Canvas().Focus(cuenta)
}

// mensajeDeEntrada traduce el fallo a algo accionable.
func mensajeDeEntrada(err error) string {
	var e *bus.Error
	if errors.As(err, &e) {
		switch {
		case e.Estado == 401:
			return "Cuenta o contraseña incorrectas."
		case e.Estado == 403:
			return "La cuenta está dada de baja. Pide a un administrador que la reactive."
		case e.Mensaje != "":
			return e.Mensaje
		}
	}
	return "No se pudo conectar con el servidor: " + err.Error()
}
