package ui

import (
	"bytes"
	"context"
	"io"
	"time"

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
		widget.NewButtonWithIcon("Subir una foto", theme.UploadIcon(), v.subir),
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
	acciones := widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), func() { v.acciones(im) })
	acciones.Importance = widget.LowImportance

	pie := container.NewBorder(nil, nil, nil, acciones, titulo)
	return container.NewBorder(nil, pie, nil, nil, container.NewStack(marco, abrir))
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

// albumPropio es donde caen las fotos que se suben desde aquí, igual que en la
// aplicación web.
const albumPropio = "Mi unidad"

// subir manda una imagen del equipo a Mi unidad y la registra en Fotos. El
// Álbum la recoge del Home por RMI: no se vuelve a subir desde aquí.
func (v *vistaFotos) subir() {
	dialog.ShowFileOpen(func(lector fyne.URIReadCloser, err error) {
		if err != nil || lector == nil {
			return
		}
		nombre := lector.URI().Name()
		espera := dialog.NewCustomWithoutButtons("Subiendo "+nombre,
			widget.NewProgressBarInfinite(), v.app.win)
		espera.Show()

		go func() {
			defer lector.Close()
			ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancelar()

			nodo, err := v.app.cli.Subir(ctx, "/", nombre, lector)
			if err == nil {
				var album bus.Album
				album, err = v.albumDeMiUnidad(ctx)
				if err == nil {
					_, err = v.app.cli.AgregarImagenDelHome(ctx, album.ID, nodo.Ruta, nombre)
				}
			}
			fyne.Do(func() {
				espera.Hide()
				if err != nil {
					v.app.error(err)
					return
				}
				v.cargarAlbums()
				v.cargarFotos()
				if v.app.refrescarCuota != nil {
					v.app.refrescarCuota()
				}
			})
		}()
	}, v.app.win)
}

// albumDeMiUnidad busca el álbum propio y lo crea si no existe.
func (v *vistaFotos) albumDeMiUnidad(ctx context.Context) (bus.Album, error) {
	albums, err := v.app.cli.Albums(ctx)
	if err != nil {
		return bus.Album{}, err
	}
	for _, a := range albums {
		if a.Titulo == albumPropio && a.MiRol == "propietario" {
			return a, nil
		}
	}
	return v.app.cli.CrearAlbum(ctx, albumPropio)
}

// acciones son las decisiones que se pueden tomar sobre una foto.
func (v *vistaFotos) acciones(im bus.Imagen) {
	var d dialog.Dialog
	ver := widget.NewButtonWithIcon("Ver la imagen", theme.VisibilityIcon(), func() {
		d.Hide()
		v.abrir(im)
	})
	ver.Importance = widget.HighImportance
	quitar := widget.NewButtonWithIcon("Quitar de Fotos", theme.DeleteIcon(), func() {
		d.Hide()
		v.quitar(im)
	})

	d = dialog.NewCustom(im.Titulo, "Cerrar",
		container.NewVBox(widget.NewLabel(detalleDeFoto(im)), ver, quitar), v.app.win)
	d.Show()
}

// quitar la saca del álbum. El archivo original sigue en Mi unidad, porque el
// Álbum guarda su propia copia de lo que recogió del Home.
func (v *vistaFotos) quitar(im bus.Imagen) {
	dialog.ShowConfirm("Quitar de Fotos",
		"¿Quitar \""+im.Titulo+"\" del álbum? El archivo sigue en Mi unidad.",
		func(ok bool) {
			if !ok {
				return
			}
			enSegundoPlano(func(ctx context.Context) (struct{}, error) {
				return struct{}{}, v.app.cli.EliminarImagen(ctx, im.ID)
			}, func(_ struct{}, err error) {
				if err != nil {
					v.app.error(err)
					return
				}
				v.cargarFotos()
			})
		}, v.app.win)
}
