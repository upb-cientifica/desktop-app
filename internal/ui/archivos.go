package ui

import (
	"context"
	"io"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaArchivos dibuja el Home: Mi unidad y sus carpetas, Destacados o
// Papelera. Las tres se listan igual y solo cambian las acciones.
func (a *App) vistaArchivos(seccion bus.Seccion) fyne.CanvasObject {
	v := &vistaDeArchivos{app: a, seccion: seccion, ruta: "/"}
	return v.construir()
}

// vistaCompartidos lista lo que otras cuentas compartieron conmigo, que llega
// como lista plana y con el propietario de cada archivo.
func (a *App) vistaCompartidos() fyne.CanvasObject {
	v := &vistaDeArchivos{app: a, compartidos: true}
	return v.construir()
}

type vistaDeArchivos struct {
	app         *App
	seccion     bus.Seccion
	compartidos bool

	ruta   string
	nodos  []bus.Nodo
	lista  *widget.List
	camino *widget.Label
	estado *widget.Label
	raiz   *fyne.Container
}

func (v *vistaDeArchivos) construir() fyne.CanvasObject {
	v.camino = widget.NewLabel("")
	v.estado = widget.NewLabel("")

	v.lista = widget.NewList(
		func() int { return len(v.nodos) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil,
				container.NewHBox(widget.NewIcon(theme.FileIcon()), widget.NewLabel("plantilla")),
				widget.NewLabel("tamaño"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			n := v.nodos[i]
			fila := o.(*fyne.Container)
			izq := fila.Objects[1].(*fyne.Container)
			izq.Objects[0].(*widget.Icon).SetResource(iconoDe(n))
			izq.Objects[1].(*widget.Label).SetText(n.Nombre)
			fila.Objects[2].(*widget.Label).SetText(detalleDe(n))
		},
	)
	v.lista.OnSelected = func(i widget.ListItemID) {
		v.lista.Unselect(i)
		if i < 0 || i >= len(v.nodos) {
			return
		}
		n := v.nodos[i]
		if n.EsCarpeta && v.seccion == bus.MiUnidad && !v.compartidos {
			v.ir(n.Ruta)
			return
		}
		v.acciones(n)
	}

	v.raiz = container.NewBorder(
		container.NewVBox(v.barra(), v.camino),
		v.estado, nil, nil,
		v.lista,
	)
	v.cargar()
	return v.raiz
}

func (v *vistaDeArchivos) barra() fyne.CanvasObject {
	botones := []fyne.CanvasObject{}

	if v.seccion == bus.MiUnidad && !v.compartidos {
		botones = append(botones,
			widget.NewButtonWithIcon("Subir archivo", theme.UploadIcon(), v.subir),
			widget.NewButtonWithIcon("Nueva carpeta", theme.FolderNewIcon(), v.nuevaCarpeta),
		)
		atras := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
			v.ir(bus.Padre(v.ruta))
		})
		botones = append([]fyne.CanvasObject{atras}, botones...)
	}
	if v.seccion == bus.Papelera {
		botones = append(botones, widget.NewButtonWithIcon("Vaciar papelera", theme.DeleteIcon(), v.vaciarPapelera))
	}
	botones = append(botones, widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), v.cargar))
	return container.NewHBox(botones...)
}

// ---------- datos ----------

func (v *vistaDeArchivos) ir(ruta string) {
	v.ruta = ruta
	v.cargar()
}

func (v *vistaDeArchivos) cargar() {
	v.estado.SetText("Cargando…")
	ruta, seccion, compartidos := v.ruta, v.seccion, v.compartidos

	enSegundoPlano(func(ctx context.Context) ([]bus.Nodo, error) {
		if compartidos {
			return v.app.cli.CompartidosConmigo(ctx)
		}
		l, err := v.app.cli.Listar(ctx, ruta, seccion)
		if err != nil {
			return nil, err
		}
		return append(append([]bus.Nodo{}, l.Carpetas...), l.Archivos...), nil
	}, func(ns []bus.Nodo, err error) {
		if err != nil {
			v.estado.SetText("No se pudo cargar: " + err.Error())
			return
		}
		v.nodos = ns
		v.lista.Refresh()
		v.camino.SetText(v.tituloDeRuta())
		if len(ns) == 0 {
			v.estado.SetText("Aquí no hay nada.")
		} else {
			v.estado.SetText(contar(ns))
		}
	})
}

func (v *vistaDeArchivos) tituloDeRuta() string {
	switch {
	case v.compartidos:
		return "Archivos que otras cuentas compartieron contigo"
	case v.seccion == bus.Destacados:
		return "Archivos y carpetas destacados"
	case v.seccion == bus.Papelera:
		return "Lo eliminado se conserva hasta vaciar la papelera"
	case v.ruta == "/":
		return "Mi unidad"
	default:
		return "Mi unidad" + v.ruta
	}
}

// ---------- acciones ----------

func (v *vistaDeArchivos) acciones(n bus.Nodo) {
	var botones []fyne.CanvasObject
	cerrar := func() {}

	añadir := func(texto string, icono fyne.Resource, fn func()) {
		botones = append(botones, widget.NewButtonWithIcon(texto, icono, func() {
			cerrar()
			fn()
		}))
	}

	if v.seccion == bus.Papelera {
		añadir("Restaurar", theme.ViewRestoreIcon(), func() { v.restaurar(n) })
		añadir("Eliminar definitivamente", theme.DeleteIcon(), func() { v.eliminar(n, true) })
	} else {
		if !n.EsCarpeta {
			añadir("Descargar", theme.DownloadIcon(), func() { v.descargar(n) })
			añadir("Ver versiones", theme.HistoryIcon(), func() { v.versiones(n) })
		}
		if !v.compartidos {
			añadir("Compartir", theme.MailSendIcon(), func() { v.compartir(n) })
			añadir("Renombrar", theme.DocumentCreateIcon(), func() { v.renombrar(n) })
			estrella := "Destacar"
			if n.Destacado {
				estrella = "Quitar de destacados"
			}
			añadir(estrella, theme.ConfirmIcon(), func() { v.destacar(n) })
			añadir("Mover a la papelera", theme.DeleteIcon(), func() { v.eliminar(n, false) })
		}
	}

	detalle := widget.NewLabel(detalleLargo(n))
	contenido := container.NewVBox(append([]fyne.CanvasObject{detalle}, botones...)...)
	d := dialog.NewCustom(n.Nombre, "Cerrar", contenido, v.app.win)
	cerrar = d.Hide
	d.Show()
}

func (v *vistaDeArchivos) subir() {
	dialog.ShowFileOpen(func(lector fyne.URIReadCloser, err error) {
		if err != nil || lector == nil {
			return
		}
		nombre := lector.URI().Name()
		destino := v.ruta
		progreso := dialog.NewCustomWithoutButtons("Subiendo "+nombre,
			widget.NewProgressBarInfinite(), v.app.win)
		progreso.Show()

		go func() {
			defer lector.Close()
			// Sin plazo de treinta segundos: un archivo grande tarda más.
			ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancelar()
			_, err := v.app.cli.Subir(ctx, destino, nombre, lector)
			fyne.Do(func() {
				progreso.Hide()
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

func (v *vistaDeArchivos) descargar(n bus.Nodo) {
	dialog.ShowFileSave(func(escritor fyne.URIWriteCloser, err error) {
		if err != nil || escritor == nil {
			return
		}
		progreso := dialog.NewCustomWithoutButtons("Descargando "+n.Nombre,
			widget.NewProgressBarInfinite(), v.app.win)
		progreso.Show()

		go func() {
			defer escritor.Close()
			ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancelar()
			cuerpo, err := v.app.cli.DescargarArchivo(ctx, n.Ruta, propietarioDe(n, v.compartidos))
			if err == nil {
				defer cuerpo.Close()
				_, err = io.Copy(escritor, cuerpo)
			}
			fyne.Do(func() {
				progreso.Hide()
				if err != nil {
					v.app.error(err)
					return
				}
				dialog.ShowInformation("Descargado", n.Nombre+" quedó en tu equipo.", v.app.win)
			})
		}()
	}, v.app.win)
}

func (v *vistaDeArchivos) nuevaCarpeta() {
	entrada := widget.NewEntry()
	entrada.SetPlaceHolder("Nombre de la carpeta")
	dialog.ShowForm("Nueva carpeta", "Crear", "Cancelar",
		[]*widget.FormItem{widget.NewFormItem("Nombre", entrada)},
		func(ok bool) {
			if !ok || entrada.Text == "" {
				return
			}
			padre := v.ruta
			enSegundoPlano(func(ctx context.Context) (bus.Nodo, error) {
				return v.app.cli.CrearCarpeta(ctx, padre, entrada.Text)
			}, v.trasCambiar)
		}, v.app.win)
}

func (v *vistaDeArchivos) renombrar(n bus.Nodo) {
	entrada := widget.NewEntry()
	entrada.SetText(n.Nombre)
	dialog.ShowForm("Renombrar", "Guardar", "Cancelar",
		[]*widget.FormItem{widget.NewFormItem("Nombre", entrada)},
		func(ok bool) {
			if !ok || entrada.Text == "" || entrada.Text == n.Nombre {
				return
			}
			enSegundoPlano(func(ctx context.Context) (bus.Nodo, error) {
				return v.app.cli.Renombrar(ctx, n.Ruta, entrada.Text)
			}, v.trasCambiar)
		}, v.app.win)
}

func (v *vistaDeArchivos) destacar(n bus.Nodo) {
	enSegundoPlano(func(ctx context.Context) (bus.Nodo, error) {
		return v.app.cli.Destacar(ctx, n.Ruta)
	}, v.trasCambiar)
}

func (v *vistaDeArchivos) restaurar(n bus.Nodo) {
	enSegundoPlano(func(ctx context.Context) (bus.Nodo, error) {
		return v.app.cli.Restaurar(ctx, n.Ruta)
	}, v.trasCambiar)
}

func (v *vistaDeArchivos) eliminar(n bus.Nodo, definitivo bool) {
	pregunta := "¿Mover \"" + n.Nombre + "\" a la papelera?"
	if definitivo {
		pregunta = "¿Eliminar \"" + n.Nombre + "\" para siempre? Esto no se puede deshacer."
	}
	dialog.ShowConfirm("Eliminar", pregunta, func(ok bool) {
		if !ok {
			return
		}
		enSegundoPlano(func(ctx context.Context) (struct{}, error) {
			return struct{}{}, v.app.cli.Eliminar(ctx, n.Ruta, definitivo)
		}, func(_ struct{}, err error) { v.trasCambiar(bus.Nodo{}, err) })
	}, v.app.win)
}

func (v *vistaDeArchivos) vaciarPapelera() {
	if len(v.nodos) == 0 {
		return
	}
	dialog.ShowConfirm("Vaciar papelera",
		"Se eliminarán para siempre los elementos de la papelera. ¿Seguir?", func(ok bool) {
			if !ok {
				return
			}
			nodos := append([]bus.Nodo{}, v.nodos...)
			enSegundoPlano(func(ctx context.Context) (struct{}, error) {
				for _, n := range nodos {
					if err := v.app.cli.Eliminar(ctx, n.Ruta, true); err != nil {
						return struct{}{}, err
					}
				}
				return struct{}{}, nil
			}, func(_ struct{}, err error) { v.trasCambiar(bus.Nodo{}, err) })
		}, v.app.win)
}

func (v *vistaDeArchivos) compartir(n bus.Nodo) {
	correo := widget.NewEntry()
	correo.SetPlaceHolder("cuenta o correo@" + bus.DominioCorreo)
	permiso := widget.NewSelect([]string{"lectura", "escritura"}, nil)
	permiso.SetSelected("lectura")

	yaCompartido := widget.NewLabel(listaDeComparticiones(n))

	dialog.ShowForm("Compartir "+n.Nombre, "Compartir", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Con", correo),
		widget.NewFormItem("Permiso", permiso),
		widget.NewFormItem("Ahora mismo", yaCompartido),
	}, func(ok bool) {
		if !ok || correo.Text == "" {
			return
		}
		enSegundoPlano(func(ctx context.Context) (struct{}, error) {
			return struct{}{}, v.app.cli.Compartir(ctx, n.Ruta, bus.ACorreo(correo.Text), permiso.Selected)
		}, func(_ struct{}, err error) {
			if err != nil {
				v.app.error(err)
				return
			}
			dialog.ShowInformation("Compartido",
				n.Nombre+" ahora está compartido con "+bus.ACorreo(correo.Text)+".", v.app.win)
			v.cargar()
		})
	}, v.app.win)
}

func (v *vistaDeArchivos) versiones(n bus.Nodo) {
	enSegundoPlano(func(ctx context.Context) ([]bus.Version, error) {
		return v.app.cli.Versiones(ctx, n.Ruta)
	}, func(vs []bus.Version, err error) {
		if err != nil {
			v.app.error(err)
			return
		}
		if len(vs) == 0 {
			dialog.ShowInformation("Versiones", "Este archivo solo tiene la versión actual.", v.app.win)
			return
		}
		texto := ""
		for _, ver := range vs {
			texto += "v" + itoa(ver.Version) + " · " + bus.Legible(ver.TamanoBytes) + " · " + ver.CreadaEn + "\n"
		}
		dialog.ShowInformation("Versiones de "+n.Nombre, texto, v.app.win)
	})
}

// trasCambiar recarga la vista y la cuota cuando una acción salió bien.
func (v *vistaDeArchivos) trasCambiar(_ bus.Nodo, err error) {
	if err != nil {
		v.app.error(err)
		return
	}
	v.cargar()
	if v.app.refrescarCuota != nil {
		v.app.refrescarCuota()
	}
}

// ---------- presentación ----------

func iconoDe(n bus.Nodo) fyne.Resource {
	switch {
	case n.EsCarpeta:
		return theme.FolderIcon()
	case n.Tipo == "imagen":
		return theme.MediaPhotoIcon()
	case n.Tipo == "video":
		return theme.MediaVideoIcon()
	default:
		return theme.FileIcon()
	}
}

func detalleDe(n bus.Nodo) string {
	if n.EsCarpeta {
		return fecha(n.ModificadoEn)
	}
	return bus.Legible(n.TamanoBytes) + " · " + fecha(n.ModificadoEn)
}

func detalleLargo(n bus.Nodo) string {
	t := n.Ruta + "\n"
	if !n.EsCarpeta {
		t += "Tamaño: " + bus.Legible(n.TamanoBytes) + " · versión " + itoa(n.Version) + "\n"
	}
	t += "Modificado: " + fecha(n.ModificadoEn) + "\n"
	t += "Propietario: " + n.Propietario + " · permisos " + n.Permisos.Octal
	return t
}

func listaDeComparticiones(n bus.Nodo) string {
	if len(n.CompartidoCon) == 0 {
		return "con nadie"
	}
	t := ""
	for i, c := range n.CompartidoCon {
		if i > 0 {
			t += ", "
		}
		t += c.Correo + " (" + c.Permiso + ")"
	}
	return t
}

func propietarioDe(n bus.Nodo, compartidos bool) string {
	if compartidos {
		return n.Propietario
	}
	return ""
}

func contar(ns []bus.Nodo) string {
	carpetas, archivos := 0, 0
	for _, n := range ns {
		if n.EsCarpeta {
			carpetas++
		} else {
			archivos++
		}
	}
	return itoa(int64(carpetas)) + " carpetas · " + itoa(int64(archivos)) + " archivos"
}

// fecha deja la marca ISO del servidor en algo corto y local.
func fecha(iso string) string {
	if iso == "" {
		return "—"
	}
	for _, formato := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(formato, iso); err == nil {
			return t.Local().Format("02/01/2006 15:04")
		}
	}
	return iso
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
