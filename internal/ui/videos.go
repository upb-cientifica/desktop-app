package ui

import (
	"context"
	"image"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/reproductor"
)

// vistaVideos lista el catálogo de Streaming y lo reproduce dentro de la
// ventana (ver internal/reproductor). Si el equipo no tiene ffmpeg, que es lo
// que descodifica, queda la salida de emergencia: abrir el flujo en el
// reproductor del sistema.
type vistaVideos struct {
	app    *App
	videos []bus.Video
	lista  *widget.List
	estado *widget.Label
}

func (a *App) vistaVideos() fyne.CanvasObject {
	v := &vistaVideos{app: a}
	v.estado = widget.NewLabel("Cargando catálogo…")

	v.lista = widget.NewList(
		func() int { return len(v.videos) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil,
				widget.NewIcon(theme.MediaVideoIcon()),
				widget.NewLabel("duración"),
				widget.NewLabel("título"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.videos) {
				return
			}
			vid := v.videos[i]
			fila := o.(*fyne.Container)
			fila.Objects[0].(*widget.Label).SetText(vid.Titulo)
			fila.Objects[2].(*widget.Label).SetText(detalleDeVideo(vid))
		},
	)
	v.lista.OnSelected = func(i widget.ListItemID) {
		v.lista.Unselect(i)
		if i >= 0 && i < len(v.videos) {
			v.acciones(v.videos[i])
		}
	}

	barra := container.NewHBox(
		widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), v.cargar),
		widget.NewButtonWithIcon("Publicar un video", theme.UploadIcon(), v.publicar),
	)
	texto := "El video se reproduce aquí mismo; el flujo HLS llega por el Service Bus, en trozos."
	if !reproductor.Disponible() {
		texto = "Para reproducir dentro de la aplicación hace falta ffmpeg en el equipo. " +
			"Mientras tanto, el video se abre en el reproductor del sistema."
	}
	nota := widget.NewLabel(texto)
	nota.Wrapping = fyne.TextWrapWord

	v.cargar()
	return container.NewBorder(container.NewVBox(barra, v.estado, nota), nil, nil, nil, v.lista)
}

func (v *vistaVideos) cargar() {
	v.estado.SetText("Cargando catálogo…")
	enSegundoPlano(v.app.cli.Videos, func(vs []bus.Video, err error) {
		if err != nil {
			v.estado.SetText("No se pudo cargar el catálogo: " + err.Error())
			return
		}
		v.videos = vs
		v.lista.Refresh()
		if len(vs) == 0 {
			v.estado.SetText("Todavía no hay videos publicados.")
		} else {
			v.estado.SetText(itoa(int64(len(vs))) + " videos")
		}
	})
}

// acciones abre el detalle del video. Si está listo se puede reproducir de
// una: el listado del servicio no dice si el empaquetado HLS terminó —eso solo
// viene en el detalle—, así que se consulta al abrirlo.
func (v *vistaVideos) acciones(vid bus.Video) {
	detalle := widget.NewLabel(detalleLargoDeVideo(vid))
	aviso := widget.NewLabel("")
	aviso.Hide()
	var d dialog.Dialog

	reproducir := widget.NewButtonWithIcon("Reproducir", theme.MediaPlayIcon(), func() {
		d.Hide()
		if reproductor.Disponible() {
			v.reproducirAqui(vid)
		} else {
			v.abrirEnElSistema(vid)
		}
	})
	reproducir.Importance = widget.HighImportance
	enElSistema := widget.NewButtonWithIcon("Abrir en el reproductor del sistema", theme.ComputerIcon(), func() {
		d.Hide()
		v.abrirEnElSistema(vid)
	})
	copiar := widget.NewButtonWithIcon("Copiar el enlace del flujo", theme.ContentCopyIcon(), func() {
		d.Hide()
		v.app.win.Clipboard().SetContent(v.app.cli.URLDelManifiesto(vid.ID))
	})
	eliminar := widget.NewButtonWithIcon("Quitar del catálogo", theme.DeleteIcon(), func() {
		d.Hide()
		v.eliminar(vid)
	})

	d = dialog.NewCustom(vid.Titulo, "Cerrar",
		container.NewVBox(detalle, aviso, reproducir, enElSistema, copiar, eliminar), v.app.win)
	d.Show()

	enSegundoPlano(func(ctx context.Context) (bus.Video, error) {
		return v.app.cli.Video(ctx, vid.ID)
	}, func(completo bus.Video, err error) {
		if err != nil {
			return // que se pueda intentar igual: el servidor dirá si no puede
		}
		detalle.SetText(detalleLargoDeVideo(completo))
		if !completo.HlsListo {
			aviso.SetText("El servidor todavía está preparando este video; puede que aún no se vea.")
			aviso.Show()
		}
	})
}

// abrirEnElSistema entrega el flujo al reproductor del equipo. El token viaja
// en la URL porque un reproductor externo no manda encabezados.
func (v *vistaVideos) abrirEnElSistema(vid bus.Video) {
	enlace, err := url.Parse(v.app.cli.URLDelManifiesto(vid.ID))
	if err != nil {
		v.app.error(err)
		return
	}
	if err := v.app.fyne.OpenURL(enlace); err != nil {
		v.app.error(err)
	}
}

// publicar sube un video del equipo a Mi unidad y lo manda a Streaming, que lo
// recoge del Home por RMI y lo empaqueta en HLS con ffmpeg. Son dos pasos
// porque el catálogo de video no guarda archivos: siempre parte del Home.
func (v *vistaVideos) publicar() {
	dialog.ShowFileOpen(func(lector fyne.URIReadCloser, err error) {
		if err != nil || lector == nil {
			return
		}
		nombre := lector.URI().Name()
		aviso := widget.NewLabel("Subiendo " + nombre + " a Mi unidad…")
		espera := dialog.NewCustomWithoutButtons("Publicando",
			container.NewVBox(aviso, widget.NewProgressBarInfinite()), v.app.win)
		espera.Show()

		go func() {
			defer lector.Close()
			// Subir y convertir tardan: plazo propio y amplio.
			ctx, cancelar := context.WithTimeout(context.Background(), time.Hour)
			defer cancelar()

			nodo, err := v.app.cli.Subir(ctx, "/", nombre, lector)
			if err != nil {
				fyne.Do(func() {
					espera.Hide()
					v.app.error(err)
				})
				return
			}
			fyne.Do(func() { aviso.SetText("Preparando el video en el servidor…") })

			_, err = v.app.cli.ImportarVideoDelHome(ctx, nodo.Ruta, sinExtension(nombre))
			fyne.Do(func() {
				espera.Hide()
				if err != nil {
					v.app.error(err)
					return
				}
				v.cargar()
				if v.app.refrescarCuota != nil {
					v.app.refrescarCuota()
				}
			})
		}()
	}, v.app.win)
}

// sinExtension deja "clase-3.mp4" como "clase-3", que es mejor título.
func sinExtension(nombre string) string {
	if i := strings.LastIndex(nombre, "."); i > 0 {
		return nombre[:i]
	}
	return nombre
}

// detalleDeVideo es la línea del listado. No dice si el HLS está listo porque
// el listado del servicio no trae ese dato: se sabe al abrir el video.
func detalleDeVideo(v bus.Video) string {
	t := duracion(v.DuracionSeg.Int64())
	if v.TamanoBytes > 0 {
		t += " · " + legible(v.TamanoBytes.Int64())
	}
	return t
}

func detalleLargoDeVideo(v bus.Video) string {
	t := "Duración: " + duracion(v.DuracionSeg.Int64()) + " · " + legible(v.TamanoBytes.Int64()) + "\n"
	if v.Autor != "" {
		t += "Autor: " + v.Autor + "\n"
	}
	if v.Proyecto != "" {
		t += "Proyecto: " + v.Proyecto + "\n"
	}
	t += "Publicado: " + fecha(v.PublicadoEn) + " · acceso " + v.NivelAcceso
	return t
}

func duracion(segundos int64) string {
	if segundos <= 0 {
		return "—"
	}
	m := segundos / 60
	s := segundos % 60
	texto := itoa(m) + ":"
	if s < 10 {
		texto += "0"
	}
	return texto + itoa(s)
}

// reproducirAqui abre el video dentro de la aplicación: ffmpeg descodifica el
// flujo que sirve el bus y la ventana pinta los cuadros según llegan.
func (v *vistaVideos) reproducirAqui(vid bus.Video) {
	const ancho = 720
	url := v.app.cli.URLDelManifiesto(vid.ID)

	lienzo := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, ancho, ancho*9/16)))
	lienzo.FillMode = canvas.ImageFillContain
	estado := widget.NewLabel("Preparando el video…")
	pausa := widget.NewButtonWithIcon("Pausa", theme.MediaPauseIcon(), nil)
	pausa.Disable()

	marco := container.NewGridWrap(fyne.NewSize(ancho, float32(ancho)*9/16), lienzo)
	contenido := container.NewBorder(nil, container.NewVBox(estado, pausa), nil, nil, marco)

	var sesion *reproductor.Sesion
	d := dialog.NewCustom(vid.Titulo, "Cerrar", contenido, v.app.win)
	d.SetOnClosed(func() {
		if sesion != nil {
			sesion.Cerrar()
		}
	})
	d.Show()

	go func() {
		ctx, cancelar := context.WithCancel(context.Background())
		defer cancelar()

		medidas, err := reproductor.Medir(ctx, url)
		if err != nil {
			// Lo normal aquí es que el servidor no tenga el video listo: o
			// sigue convirtiéndolo, o la conversión falló y no hay nada que
			// leer. El detalle técnico no le dice nada a quien mira.
			fyne.Do(func() {
				estado.SetText("El servidor no tiene este video listo para reproducir. " +
					"Si acabas de publicarlo, espera un momento y vuelve a intentarlo.")
			})
			return
		}
		s, err := reproductor.Abrir(ctx, url, ancho, medidas)
		if err != nil {
			fyne.Do(func() { estado.SetText("No se pudo reproducir: " + err.Error()) })
			return
		}
		sesion = s

		fyne.Do(func() {
			pausa.Enable()
			pausa.OnTapped = func() {
				if s.EnPausa() {
					s.Reanudar()
					pausa.SetText("Pausa")
					pausa.SetIcon(theme.MediaPauseIcon())
				} else {
					s.Pausar()
					pausa.SetText("Reanudar")
					pausa.SetIcon(theme.MediaPlayIcon())
				}
			}
		})

		for cuadro := range s.Cuadros {
			img := cuadro
			fyne.Do(func() {
				lienzo.Image = img
				lienzo.Refresh()
				estado.SetText(reloj(s.Transcurrido()) + " / " + reloj(medidas.Duracion))
			})
		}

		if err := <-s.Terminado; err != nil {
			fyne.Do(func() { estado.SetText("La reproducción se cortó: " + err.Error()) })
			return
		}
		fyne.Do(func() { estado.SetText("Fin del video.") })
	}()
}

// reloj deja una duración como 1:52.
func reloj(d time.Duration) string {
	return duracion(int64(d / time.Second))
}

func (v *vistaVideos) eliminar(vid bus.Video) {
	dialog.ShowConfirm("Quitar del catálogo",
		"¿Quitar \""+vid.Titulo+"\" de Videos? El archivo original sigue en Mi unidad.",
		func(ok bool) {
			if !ok {
				return
			}
			enSegundoPlano(func(ctx context.Context) (struct{}, error) {
				return struct{}{}, v.app.cli.EliminarVideo(ctx, vid.ID)
			}, func(_ struct{}, err error) {
				if err != nil {
					v.app.error(err)
					return
				}
				v.cargar()
			})
		}, v.app.win)
}
