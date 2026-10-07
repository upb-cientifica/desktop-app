package ui

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// presentacion mantiene todo el estado y la caché en el hilo de la interfaz.
// Las descargas y el reloj solo entregan resultados mediante fyne.Do.
func (a *App) presentacion(fotos []bus.Imagen) {
	if len(fotos) == 0 {
		return
	}
	fotos = append([]bus.Imagen(nil), fotos...)
	ventana := a.fyne.NewWindow("Presentación")
	ventana.SetFullScreen(true)
	contexto, cancelar := context.WithCancel(context.Background())
	reloj := time.NewTicker(5 * time.Second)
	cerrado, pausado := false, false
	indice := 0
	cache := map[string][]byte{}
	pendientes := map[string]bool{}
	marco := container.NewStack()
	titulo := canvas.NewText("", color.White)
	titulo.Alignment = fyne.TextAlignCenter
	mensaje := canvas.NewText("Cargando imagen…", color.White)
	mensaje.Alignment = fyne.TextAlignCenter
	var mostrar func()
	var cargar func(string)
	cargar = func(id string) {
		if pendientes[id] || cache[id] != nil {
			return
		}
		pendientes[id] = true
		go func() {
			ctx, terminar := bus.ConPlazo(contexto)
			defer terminar()
			cuerpo, err := a.cli.ImagenCompleta(ctx, id)
			var datos []byte
			if err == nil {
				datos, err = io.ReadAll(cuerpo)
				cuerpo.Close()
			}
			fyne.Do(func() {
				if cerrado {
					return
				}
				delete(pendientes, id)
				actual := fotos[indice].ID
				siguiente := fotos[(indice+1)%len(fotos)].ID
				if err != nil {
					if id == actual {
						mensaje.Text = "No se pudo cargar la imagen: " + err.Error()
						mensaje.Refresh()
					}
					return
				}
				// Solo se conservan la imagen actual y la siguiente.
				if id != actual && id != siguiente {
					return
				}
				cache[id] = datos
				if id == actual {
					mostrar()
				}
			})
		}()
	}
	mostrar = func() {
		if cerrado {
			return
		}
		foto := fotos[indice]
		siguiente := fotos[(indice+1)%len(fotos)].ID
		for id := range cache {
			if id != foto.ID && id != siguiente {
				delete(cache, id)
			}
		}
		titulo.Text = fmt.Sprintf("%s · %d / %d", foto.Titulo, indice+1, len(fotos))
		titulo.Refresh()
		if datos := cache[foto.ID]; datos != nil {
			imagen := canvas.NewImageFromReader(bytes.NewReader(datos), foto.ID)
			imagen.FillMode = canvas.ImageFillContain
			marco.Objects = []fyne.CanvasObject{imagen}
		} else {
			mensaje.Text = "Cargando imagen…"
			mensaje.Refresh()
			marco.Objects = []fyne.CanvasObject{container.NewCenter(mensaje)}
			cargar(foto.ID)
		}
		marco.Refresh()
		cargar(siguiente)
	}
	avanzar := func(paso int) {
		if cerrado {
			return
		}
		indice = (indice + paso + len(fotos)) % len(fotos)
		mostrar()
	}
	var pausa *widget.Button
	alternar := func() {
		pausado = !pausado
		if pausado {
			pausa.SetText("Reproducir")
		} else {
			pausa.SetText("Pausar")
		}
	}
	pausa = widget.NewButton("Pausar", alternar)
	controles := container.NewCenter(container.NewHBox(
		widget.NewButton("Anterior", func() { avanzar(-1) }), pausa,
		widget.NewButton("Siguiente", func() { avanzar(1) }),
		widget.NewButton("Cerrar", ventana.Close),
	))
	ventana.SetContent(container.NewStack(canvas.NewRectangle(color.Black),
		container.NewBorder(nil, container.NewVBox(titulo, controles), nil, nil, marco)))
	ventana.Canvas().SetOnTypedKey(func(tecla *fyne.KeyEvent) {
		switch tecla.Name {
		case fyne.KeyLeft:
			avanzar(-1)
		case fyne.KeyRight:
			avanzar(1)
		case fyne.KeySpace:
			alternar()
		case fyne.KeyEscape:
			ventana.Close()
		}
	})
	ventana.SetOnClosed(func() {
		cerrado = true
		reloj.Stop()
		cancelar()
	})
	mostrar()
	ventana.Show()
	go func() {
		for {
			select {
			case <-contexto.Done():
				return
			case <-reloj.C:
				fyne.Do(func() {
					if !cerrado && !pausado {
						avanzar(1)
					}
				})
			}
		}
	}()
}
