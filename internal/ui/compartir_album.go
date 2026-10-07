package ui

import (
	"context"
	"net/mail"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// compartirAlbum refleja cada respuesta del servicio en el diálogo y la vista.
func (a *App) compartirAlbum(album bus.Album, alCambiar func(bus.Album)) {
	personas := container.NewVBox()
	correo := widget.NewEntry()
	correo.SetPlaceHolder("usuario@upb.edu.co")
	estado := widget.NewLabel("")
	estado.Wrapping = fyne.TextWrapWord
	enlace := widget.NewEntry()
	enlace.Disable()
	confirmacion := widget.NewLabel("")
	copiar := widget.NewButton("Copiar", func() {
		a.win.Clipboard().SetContent(album.URLPublica)
		confirmacion.SetText("Enlace copiado.")
	})
	ocupado, pintando := false, false
	var pintar func()
	var activo *widget.Check
	var anadir *widget.Button
	cambiar := func(trabajo func(context.Context) (bus.Album, error)) {
		if ocupado {
			return
		}
		ocupado = true
		estado.SetText("")
		pintar()
		enSegundoPlano(trabajo, func(actualizado bus.Album, err error) {
			ocupado = false
			if err != nil {
				estado.SetText(err.Error())
			} else {
				album = actualizado
				correo.SetText("")
				alCambiar(album)
			}
			pintar()
		})
	}
	activo = widget.NewCheck("Enlace público activo", func(valor bool) {
		if pintando || ocupado {
			return
		}
		id := album.ID
		cambiar(func(ctx context.Context) (bus.Album, error) { return a.cli.PublicarAlbum(ctx, id, valor) })
	})
	anadir = widget.NewButton("Añadir", func() {
		direccion := strings.ToLower(bus.ACorreo(correo.Text))
		analizada, err := mail.ParseAddress(direccion)
		if err != nil || analizada.Address != direccion || !strings.HasSuffix(direccion, "@upb.edu.co") {
			estado.SetText("Escribe un correo válido de @upb.edu.co.")
			return
		}
		id := album.ID
		cambiar(func(ctx context.Context) (bus.Album, error) { return a.cli.CompartirAlbum(ctx, id, direccion, true) })
	})
	pintar = func() {
		pintando = true
		defer func() { pintando = false }()
		personas.Objects = nil
		for _, direccion := range album.CompartidoCon {
			quitar := widget.NewButton("Quitar", func() {
				id := album.ID
				cambiar(func(ctx context.Context) (bus.Album, error) { return a.cli.CompartirAlbum(ctx, id, direccion, false) })
			})
			if ocupado {
				quitar.Disable()
			}
			personas.Add(container.NewBorder(nil, nil, nil, quitar, widget.NewLabel(direccion)))
		}
		personas.Refresh()
		activo.SetChecked(album.Publico)
		enlace.SetText(album.URLPublica)
		confirmacion.SetText("")
		if album.Publico {
			enlace.Show()
			copiar.Show()
			if album.URLPublica == "" {
				copiar.Disable()
			} else {
				copiar.Enable()
			}
		} else {
			enlace.Hide()
			copiar.Hide()
		}
		if ocupado {
			activo.Disable()
			anadir.Disable()
		} else {
			activo.Enable()
			anadir.Enable()
		}
	}
	pintar()
	nota := widget.NewLabel("Cualquiera con este enlace puede ver el álbum sin iniciar sesión. Desactívalo para revocarlo.")
	nota.Wrapping = fyne.TextWrapWord
	lista := container.NewVScroll(personas)
	lista.SetMinSize(fyne.NewSize(460, 180))
	pestañas := container.NewAppTabs(
		container.NewTabItem("Personas", container.NewBorder(nil, container.NewBorder(nil, nil, nil, anadir, correo), nil, nil, lista)),
		container.NewTabItem("Enlace público", container.NewVBox(activo, enlace, copiar, confirmacion, nota)),
	)
	d := dialog.NewCustom("Compartir álbum: "+album.Titulo, "Cerrar", container.NewBorder(nil, estado, nil, nil, pestañas), a.win)
	d.Resize(fyne.NewSize(560, 380))
	d.Show()
}
