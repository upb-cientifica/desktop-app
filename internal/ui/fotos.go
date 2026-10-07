package ui

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
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
	app       *App
	albums    []bus.Album
	fotos     []bus.Imagen
	rejilla   *fyne.Container
	estado    *widget.Label
	filtro    *widget.Select
	albumID   string
	texto     string
	solicitud uint64
	visibles  []bus.Imagen
	etiqueta  *widget.Select
	compartir *widget.Button
	presentar *widget.Button
}

func (a *App) vistaFotos() armada {
	v := &vistaFotos{app: a}
	v.estado = widget.NewLabel("Cargando fotos…")
	v.rejilla = container.NewGridWrap(fyne.NewSize(180, 200))
	v.etiqueta = widget.NewSelect([]string{"Todas"}, func(string) { v.pintarFotos() })
	v.etiqueta.Selected = "Todas"
	v.compartir = widget.NewButton("Compartir álbum", func() {
		for _, album := range v.albums {
			if album.ID == v.albumID {
				a.compartirAlbum(album, func(actualizado bus.Album) {
					for i := range v.albums {
						if v.albums[i].ID == actualizado.ID {
							v.albums[i] = actualizado
						}
					}
					v.actualizarBarra()
				})
				break
			}
		}
	})
	v.presentar = widget.NewButton("Presentación", func() { a.presentacion(v.visibles) })
	v.actualizarBarra()
	v.filtro = widget.NewSelect([]string{"Todas"}, func(sel string) {
		v.albumID = ""
		for _, al := range v.albums {
			if al.Titulo+" ("+al.ID+")" == sel {
				v.albumID = al.ID
			}
		}
		v.actualizarBarra()
		v.cargarFotos()
	})
	v.filtro.Selected = "Todas"

	barra := container.NewHBox(
		widget.NewLabel("Álbum:"), v.filtro,
		widget.NewLabel("Etiqueta:"), v.etiqueta,
		widget.NewButtonWithIcon("Subir una foto", theme.UploadIcon(), v.subir),
		widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), func() {
			v.cargarAlbums()
			v.cargarFotos()
		}),
	)

	v.cargarAlbums()
	v.cargarFotos()
	return armada{
		vista:     container.NewBorder(container.NewVBox(barra, container.NewHBox(v.compartir, v.presentar), v.estado), nil, nil, nil, container.NewScroll(v.rejilla)),
		refrescar: func() { v.cargarAlbums(); v.cargarFotos() },
		buscar: func(texto string) {
			v.texto = strings.TrimSpace(texto)
			if v.albumID == "" {
				v.cargarFotos()
			} else {
				v.pintarFotos()
			}
		},
	}
}

func (v *vistaFotos) actualizarBarra() {
	v.compartir.Hide()
	for _, album := range v.albums {
		if album.ID == v.albumID && (album.MiRol == "propietario" || album.MiRol == "admin") {
			v.compartir.Show()
		}
	}
}

func (v *vistaFotos) cargarAlbums() {
	enSegundoPlano(v.app.cli.Albums, func(as []bus.Album, err error) {
		if err != nil {
			return
		}
		v.albums = as
		opciones := []string{"Todas"}
		for _, a := range as {
			opciones = append(opciones, a.Titulo+" ("+a.ID+")")
		}
		v.filtro.Options = opciones
		v.filtro.Refresh()
		v.actualizarBarra()
	})
}

func (v *vistaFotos) cargarFotos() {
	v.solicitud++
	solicitud := v.solicitud
	v.estado.SetText("Cargando fotos…")
	v.presentar.Disable()
	v.fotos = nil
	v.visibles = nil
	v.rejilla.Objects = nil
	v.rejilla.Refresh()
	album, texto := v.albumID, v.texto
	enSegundoPlano(func(ctx context.Context) ([]bus.Imagen, error) {
		if album == "" {
			return v.app.cli.BuscarFotos(ctx, texto, "")
		}
		return v.app.cli.Fotos(ctx, album)
	}, func(ims []bus.Imagen, err error) {
		if solicitud != v.solicitud {
			return
		}
		if err != nil {
			v.fotos = nil
			v.pintarFotos()
			v.estado.SetText("No se pudieron cargar las fotos: " + err.Error())
			return
		}
		v.fotos = ims
		v.pintarFotos()
	})
}

// pintarFotos filtra el álbum localmente y conserva las etiquetas disponibles
// antes de aplicar el filtro de etiqueta, para poder cambiarlo sin perder opciones.
func (v *vistaFotos) pintarFotos() {
	candidatas := []bus.Imagen{}
	etiquetas := map[string]bool{}
	for _, im := range v.fotos {
		texto := strings.ToLower(im.Titulo + " " + im.Descripcion + " " + strings.Join(im.Etiquetas, " "))
		if v.albumID != "" && !strings.Contains(texto, strings.ToLower(v.texto)) {
			continue
		}
		candidatas = append(candidatas, im)
		for _, etiqueta := range im.Etiquetas {
			if etiqueta != "" {
				etiquetas[etiqueta] = true
			}
		}
	}
	opciones := []string{}
	for etiqueta := range etiquetas {
		opciones = append(opciones, etiqueta)
	}
	sort.Strings(opciones)
	v.etiqueta.Options = append([]string{"Todas"}, opciones...)
	if !etiquetas[v.etiqueta.Selected] {
		v.etiqueta.Selected = "Todas"
	}
	v.etiqueta.Refresh()
	v.visibles = nil
	v.rejilla.Objects = nil
	for _, im := range candidatas {
		if v.etiqueta.Selected != "Todas" && !im.Etiquetas.Contiene(v.etiqueta.Selected) {
			continue
		}
		v.visibles = append(v.visibles, im)
		v.rejilla.Add(v.tarjeta(im))
	}
	v.rejilla.Refresh()
	v.estado.SetText(itoa(int64(len(v.visibles))) + " fotos")
	if len(v.visibles) == 0 {
		v.presentar.Disable()
	} else {
		v.presentar.Enable()
	}
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

	etiquetas := widget.NewLabel(strings.Join(im.Etiquetas, " · "))
	etiquetas.Truncation = fyne.TextTruncateEllipsis
	pie := container.NewBorder(nil, nil, nil, acciones, container.NewVBox(titulo, etiquetas))
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
	quitar := widget.NewButtonWithIcon("Mover a la papelera", theme.DeleteIcon(), func() {
		d.Hide()
		v.quitar(im)
	})

	editar := widget.NewButton("Editar detalles", func() { d.Hide(); v.editar(im) })
	d = dialog.NewCustom(im.Titulo, "Cerrar",
		container.NewVBox(widget.NewLabel(detalleDeFoto(im)), ver, editar, quitar), v.app.win)
	d.Show()
}

// quitar la saca del álbum y manda a la papelera el archivo del Home del que
// salió, para que se pueda recuperar desde ahí.
func (v *vistaFotos) quitar(im bus.Imagen) {
	dialog.ShowConfirm("Mover a la papelera",
		"¿Mover \""+im.Titulo+"\" a la papelera? Deja de estar en Fotos y el archivo "+
			"se puede recuperar desde la Papelera.",
		func(ok bool) {
			if !ok {
				return
			}
			enSegundoPlano(func(ctx context.Context) (struct{}, error) {
				if err := v.app.cli.EliminarImagen(ctx, im.ID); err != nil {
					return struct{}{}, err
				}
				return struct{}{}, v.app.aLaPapelera(ctx, im.OrigenHome)
			}, func(_ struct{}, err error) {
				if err != nil {
					v.app.error(err)
					return
				}
				v.cargarFotos()
			})
		}, v.app.win)
}

// editar conserva el formulario abierto si el servicio rechaza los cambios.
func (v *vistaFotos) editar(im bus.Imagen) {
	titulo := widget.NewEntry()
	titulo.SetText(im.Titulo)
	descripcion := widget.NewMultiLineEntry()
	descripcion.SetText(im.Descripcion)
	etiquetas := widget.NewEntry()
	etiquetas.SetText(strings.Join(im.Etiquetas, ", "))
	estado := widget.NewLabel("")
	estado.Wrapping = fyne.TextWrapWord
	var d dialog.Dialog
	var guardar *widget.Button
	guardar = widget.NewButton("Guardar", func() {
		nombre, detalle := strings.TrimSpace(titulo.Text), descripcion.Text
		lista := normalizarEtiquetas(etiquetas.Text)
		guardar.Disable()
		enSegundoPlano(func(ctx context.Context) (bus.Imagen, error) {
			return v.app.cli.ActualizarImagen(ctx, im.ID, nombre, detalle, lista)
		}, func(_ bus.Imagen, err error) {
			guardar.Enable()
			if err != nil {
				estado.SetText(err.Error())
				return
			}
			d.Hide()
			v.cargarFotos()
		})
	})
	d = dialog.NewCustom("Editar detalles", "Cancelar", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Título", titulo), widget.NewFormItem("Descripción", descripcion),
			widget.NewFormItem("Etiquetas", etiquetas)), estado, guardar), v.app.win)
	d.Resize(fyne.NewSize(520, 360))
	d.Show()
}

func normalizarEtiquetas(texto string) []string {
	resultado := []string{}
	vistas := map[string]bool{}
	for _, parte := range strings.Split(texto, ",") {
		etiqueta := strings.ToLower(strings.TrimSpace(parte))
		if etiqueta != "" && !vistas[etiqueta] {
			resultado = append(resultado, etiqueta)
			vistas[etiqueta] = true
		}
	}
	return resultado
}
