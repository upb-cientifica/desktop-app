package ui

import (
	"context"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaVideos lista el catálogo de Streaming.
//
// La reproducción no se dibuja aquí: Fyne no trae reproductor de video, y
// escribir uno para HLS sería rehacer lo que el sistema ya tiene. Al pulsar
// Reproducir se abre el flujo HLS en el reproductor del equipo (en macOS,
// QuickTime o Safari; en Linux, VLC o mpv). El video sigue viniendo del
// servidor por el bus, en trozos: no se descarga entero.
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
		widget.NewButtonWithIcon("Publicar uno de Mi unidad", theme.UploadIcon(), v.publicar),
	)
	nota := widget.NewLabel("El video se abre en el reproductor del equipo; el flujo HLS llega por el Service Bus, en trozos.")
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
		v.reproducir(vid)
	})
	reproducir.Importance = widget.HighImportance
	copiar := widget.NewButtonWithIcon("Copiar el enlace del flujo", theme.ContentCopyIcon(), func() {
		d.Hide()
		v.app.win.Clipboard().SetContent(v.app.cli.URLDelManifiesto(vid.ID))
	})

	d = dialog.NewCustom(vid.Titulo, "Cerrar",
		container.NewVBox(detalle, aviso, reproducir, copiar), v.app.win)
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

// reproducir abre el flujo HLS en el reproductor del sistema. El token viaja
// en la URL porque un reproductor externo no manda encabezados.
func (v *vistaVideos) reproducir(vid bus.Video) {
	enlace, err := url.Parse(v.app.cli.URLDelManifiesto(vid.ID))
	if err != nil {
		v.app.error(err)
		return
	}
	if err := v.app.fyne.OpenURL(enlace); err != nil {
		v.app.error(err)
	}
}

// publicar toma un video que ya está en el Home y lo manda a Streaming, que lo
// recoge por RMI y lo empaqueta en HLS con ffmpeg.
func (v *vistaVideos) publicar() {
	ruta := widget.NewEntry()
	ruta.SetPlaceHolder("/carpeta/video.mp4")
	titulo := widget.NewEntry()

	dialog.ShowForm("Publicar un video de Mi unidad", "Publicar", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Ruta en Mi unidad", ruta),
		widget.NewFormItem("Título", titulo),
	}, func(ok bool) {
		if !ok || ruta.Text == "" {
			return
		}
		espera := dialog.NewCustomWithoutButtons("Preparando el video",
			widget.NewProgressBarInfinite(), v.app.win)
		espera.Show()
		enSegundoPlano(func(ctx context.Context) (bus.Video, error) {
			// Convertir a HLS tarda: plazo propio y amplio.
			ctx, cancelar := context.WithCancel(ctx)
			defer cancelar()
			return v.app.cli.ImportarVideoDelHome(ctx, ruta.Text, titulo.Text)
		}, func(_ bus.Video, err error) {
			espera.Hide()
			if err != nil {
				v.app.error(err)
				return
			}
			v.cargar()
		})
	}, v.app.win)
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
