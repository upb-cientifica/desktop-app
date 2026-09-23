package ui

import (
	"bytes"
	"context"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaFotos muestra el Álbum de fotos: los álbumes a los que la cuenta tiene
// acceso y sus imágenes. Las miniaturas las sirve el propio servicio, así que
// aquí solo se piden y se pintan.
type vistaFotos struct {
	app     *App
	albums  []bus.Album
	fotos   []bus.Imagen
	rejilla *fyne.Container
	estado  *widget.Label
	filtro  *widget.Select
	albumID string
}

func (a *App) vistaFotos() fyne.CanvasObject {
	v := &vistaFotos{app: a}
	v.estado = widget.NewLabel("Cargando fotos…")
	v.rejilla = container.NewGridWrap(fyne.NewSize(180, 200))
	v.filtro = widget.NewSelect([]string{"Todas"}, func(sel string) {
		v.albumID = ""
		for _, al := range v.albums {
			if al.Titulo == sel {
				v.albumID = al.ID
			}
		}
		v.cargarFotos()
	})
	v.filtro.SetSelected("Todas")

	barra := container.NewHBox(
		widget.NewLabel("Álbum:"), v.filtro,
		widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), func() {
			v.cargarAlbums()
			v.cargarFotos()
		}),
	)

	v.cargarAlbums()
	v.cargarFotos()
	return container.NewBorder(container.NewVBox(barra, v.estado), nil, nil, nil,
		container.NewScroll(v.rejilla))
}

func (v *vistaFotos) cargarAlbums() {
	enSegundoPlano(v.app.cli.Albums, func(as []bus.Album, err error) {
		if err != nil {
			return
		}
		v.albums = as
		opciones := []string{"Todas"}
		for _, a := range as {
			opciones = append(opciones, a.Titulo)
		}
		v.filtro.Options = opciones
		v.filtro.Refresh()
	})
}

func (v *vistaFotos) cargarFotos() {
	v.estado.SetText("Cargando fotos…")
	album := v.albumID
	enSegundoPlano(func(ctx context.Context) ([]bus.Imagen, error) {
		return v.app.cli.Fotos(ctx, album)
	}, func(ims []bus.Imagen, err error) {
		if err != nil {
			v.estado.SetText("No se pudieron cargar las fotos: " + err.Error())
			return
		}
		v.fotos = ims
		v.rejilla.Objects = nil
		for _, im := range ims {
			v.rejilla.Add(v.tarjeta(im))
		}
		v.rejilla.Refresh()
		if len(ims) == 0 {
			v.estado.SetText("No hay fotos todavía. Sube una imagen a Mi unidad y aparecerá aquí.")
		} else {
			v.estado.SetText(itoa(int64(len(ims))) + " fotos")
		}
	})
}

// tarjeta pinta una miniatura con su título; la imagen llega después, en
// cuanto el servicio la entrega.
func (v *vistaFotos) tarjeta(im bus.Imagen) fyne.CanvasObject {
	marco := container.NewStack(widget.NewLabel("…"))
	titulo := widget.NewLabel(im.Titulo)
	titulo.Truncation = fyne.TextTruncateEllipsis

	enSegundoPlano(func(ctx context.Context) ([]byte, error) {
		cuerpo, err := v.app.cli.Miniatura(ctx, im.ID)
		if err != nil {
			return nil, err
		}
		defer cuerpo.Close()
		return io.ReadAll(cuerpo)
	}, func(datos []byte, err error) {
		if err != nil {
			marco.Objects = []fyne.CanvasObject{widget.NewIcon(theme.BrokenImageIcon())}
			marco.Refresh()
			return
		}
		img := canvas.NewImageFromReader(bytes.NewReader(datos), im.ID)
		img.FillMode = canvas.ImageFillContain
		marco.Objects = []fyne.CanvasObject{img}
		marco.Refresh()
	})

	abrir := widget.NewButton("", func() { v.abrir(im) })
	abrir.Importance = widget.LowImportance
	return container.NewBorder(nil, titulo, nil, nil, container.NewStack(marco, abrir))
}

// abrir muestra la imagen completa, que es otra llamada al servicio.
func (v *vistaFotos) abrir(im bus.Imagen) {
	cargando := widget.NewProgressBarInfinite()
	marco := container.NewStack(cargando)
	d := dialog.NewCustom(im.Titulo, "Cerrar",
		container.NewGridWrap(fyne.NewSize(820, 560), marco), v.app.win)
	d.Show()

	enSegundoPlano(func(ctx context.Context) ([]byte, error) {
		cuerpo, err := v.app.cli.ImagenCompleta(ctx, im.ID)
		if err != nil {
			return nil, err
		}
		defer cuerpo.Close()
		return io.ReadAll(cuerpo)
	}, func(datos []byte, err error) {
		if err != nil {
			marco.Objects = []fyne.CanvasObject{widget.NewLabel("No se pudo abrir: " + err.Error())}
			marco.Refresh()
			return
		}
		img := canvas.NewImageFromReader(bytes.NewReader(datos), im.ID+"-completa")
		img.FillMode = canvas.ImageFillContain
		pie := widget.NewLabel(detalleDeFoto(im))
		marco.Objects = []fyne.CanvasObject{container.NewBorder(nil, pie, nil, nil, img)}
		marco.Refresh()
	})
}

func detalleDeFoto(im bus.Imagen) string {
	t := itoa(im.Ancho.Int64()) + "×" + itoa(im.Alto.Int64()) + " · " + legible(im.TamanoBytes.Int64())
	if im.SubidaEn != "" {
		t += " · " + fecha(im.SubidaEn)
	}
	if im.OrigenHome != "" {
		t += " · viene de " + im.OrigenHome
	}
	return t
}
