package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaTrabajos habla con el clúster HPC. Del otro lado no hay REST: el bus
// convierte estas llamadas en invocaciones sobre el objeto remoto ClusterHpc
// por Java RMI, y la ventana no se entera.
type vistaTrabajos struct {
	app      *App
	trabajos []bus.Trabajo
	lista    *widget.List
	estado   *widget.Label
	// refrescando evita programar dos refrescos automáticos a la vez.
	refrescando bool
}

func (a *App) vistaTrabajos() (fyne.CanvasObject, func()) {
	v := &vistaTrabajos{app: a}
	v.estado = widget.NewLabel("Consultando el clúster…")

	v.lista = widget.NewList(
		func() int { return len(v.trabajos) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil,
				widget.NewIcon(theme.ComputerIcon()),
				container.NewHBox(widget.NewLabel("estado"),
					widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)),
				widget.NewLabel("nombre"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.trabajos) {
				return
			}
			t := v.trabajos[i]
			fila := o.(*fyne.Container)
			fila.Objects[0].(*widget.Label).SetText(t.Nombre)
			derecha := fila.Objects[2].(*fyne.Container)
			derecha.Objects[0].(*widget.Label).SetText(estadoLegible(t))
			derecha.Objects[1].(*widget.Button).OnTapped = func() { v.detalle(t) }
		},
	)
	v.lista.OnSelected = func(i widget.ListItemID) {
		v.lista.Unselect(i)
		if i >= 0 && i < len(v.trabajos) {
			v.detalle(v.trabajos[i])
		}
	}

	barra := container.NewHBox(
		widget.NewButtonWithIcon("Enviar trabajo", theme.MailSendIcon(), v.enviar),
		widget.NewButtonWithIcon("Nodos", theme.InfoIcon(), v.nodos),
		widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), v.cargar),
	)
	v.cargar()
	return container.NewBorder(container.NewVBox(barra, v.estado), nil, nil, nil, v.lista), v.cargar
}

func (v *vistaTrabajos) cargar() {
	v.estado.SetText("Consultando el clúster…")
	enSegundoPlano(v.app.cli.Trabajos, func(ts []bus.Trabajo, err error) {
		if err != nil {
			v.estado.SetText("El clúster no responde: " + err.Error())
			return
		}
		v.trabajos = ts
		v.lista.Refresh()
		if len(ts) == 0 {
			v.estado.SetText("No hay trabajos enviados.")
		} else {
			v.estado.SetText(itoa(int64(len(ts))) + " trabajos")
		}
		v.refrescarMientrasCorran()
	})
}

// refrescarMientrasCorran vuelve a pedir la lista cada 5 s mientras haya algún
// trabajo en cola o ejecutándose, para que su estado cambie sin pulsar nada.
func (v *vistaTrabajos) refrescarMientrasCorran() {
	if v.refrescando || !hayActivos(v.trabajos) {
		return
	}
	v.refrescando = true
	go func() {
		time.Sleep(5 * time.Second)
		fyne.Do(func() {
			v.refrescando = false
			v.cargar()
		})
	}()
}

func hayActivos(ts []bus.Trabajo) bool {
	for _, t := range ts {
		if !terminado(t) {
			return true
		}
	}
	return false
}

func terminado(t bus.Trabajo) bool {
	return t.Estado == "COMPLETADO" || t.Estado == "FALLIDO" || t.Estado == "CANCELADO"
}

func (v *vistaTrabajos) enviar() {
	nombre := widget.NewEntry()
	comando := widget.NewEntry()
	comando.SetPlaceHolder("./kmeans -k 10 -z -a USCensus1990.data.txt")
	rutaHome := widget.NewEntry()
	rutaHome.SetPlaceHolder("/HPC/kmeans-censo")
	procesos := widget.NewEntry()
	procesos.SetText("1")
	itemProcesos := widget.NewFormItem("Procesos", procesos)
	itemProcesos.HintText = "Slots disponibles en el clúster: consultando…"

	mostrarFormulario("Enviar trabajo MPI", "Enviar", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Nombre", nombre),
		widget.NewFormItem("Comando", comando),
		widget.NewFormItem("Carpeta en Mi unidad", rutaHome),
		itemProcesos,
	}, func(ok bool) {
		if !ok || nombre.Text == "" || comando.Text == "" {
			return
		}
		n, err := strconv.Atoi(procesos.Text)
		if err != nil || n < 1 {
			n = 1
		}
		enSegundoPlano(func(ctx context.Context) (bus.Trabajo, error) {
			return v.app.cli.EnviarTrabajo(ctx, nombre.Text, comando.Text, rutaHome.Text, n)
		}, func(_ bus.Trabajo, err error) {
			if err != nil {
				v.app.error(err)
				return
			}
			v.cargar()
		})
	}, v.app.win)

	// Por defecto, tantos procesos como slots tenga libres el clúster.
	enSegundoPlano(v.app.cli.SlotsDisponibles, func(slots int, err error) {
		if err != nil || slots < 1 {
			itemProcesos.HintText = "Slots disponibles en el clúster: sin datos"
		} else {
			itemProcesos.HintText = "Slots disponibles en el clúster: " + itoa(int64(slots))
			procesos.SetText(itoa(int64(slots)))
		}
		procesos.Refresh()
	})
}

// detalle muestra un trabajo y se actualiza solo cada 3 s mientras el diálogo
// esté abierto y el trabajo no haya terminado.
func (v *vistaTrabajos) detalle(t bus.Trabajo) {
	datos := widget.NewLabel(detalleLargoDeTrabajo(t))
	progreso := widget.NewProgressBar()
	progreso.SetValue(float64(t.Progreso.Int64()) / 100)
	avance := widget.NewLabel("")
	salida := widget.NewMultiLineEntry()
	salida.Wrapping = fyne.TextWrapOff
	salida.SetText("Pidiendo la salida…")

	var d dialog.Dialog
	cancelar := widget.NewButtonWithIcon("Cancelar el trabajo", theme.CancelIcon(), func() {
		d.Hide()
		enSegundoPlano(func(ctx context.Context) (struct{}, error) {
			return struct{}{}, v.app.cli.CancelarTrabajo(ctx, t.ID)
		}, func(_ struct{}, err error) {
			if err != nil {
				v.app.error(err)
				return
			}
			v.cargar()
		})
	})
	resultados := widget.NewButtonWithIcon("Abrir resultados", theme.FolderOpenIcon(), nil)
	resultados.Importance = widget.HighImportance
	resultados.Hide()

	abierto := true
	var pintar func(bus.Trabajo, string)
	var pedir func()
	pintar = func(t bus.Trabajo, texto string) {
		datos.SetText(detalleLargoDeTrabajo(t))
		progreso.SetValue(float64(t.Progreso.Int64()) / 100)
		lineas := lineasVisibles(texto)
		avance.SetText(resumenDeAvance(t, texto))
		if len(lineas) == 0 {
			salida.SetText("(sin salida todavía)")
		} else {
			salida.SetText(strings.Join(lineas, "\n"))
			salida.CursorRow = len(lineas) - 1
			salida.Refresh()
		}
		if terminado(t) {
			cancelar.Disable()
			v.buscarResultados(t, resultados, d)
		}
	}
	pedir = func() {
		enSegundoPlano(func(ctx context.Context) (struct {
			t     bus.Trabajo
			texto string
		}, error) {
			var r struct {
				t     bus.Trabajo
				texto string
			}
			var err error
			if r.t, err = v.app.cli.Trabajo(ctx, t.ID); err != nil {
				return r, err
			}
			r.texto, err = v.app.cli.SalidaDelTrabajo(ctx, t.ID)
			return r, err
		}, func(r struct {
			t     bus.Trabajo
			texto string
		}, err error) {
			if !abierto {
				return
			}
			if err != nil {
				salida.SetText("No se pudo leer el trabajo: " + err.Error())
				return
			}
			pintar(r.t, r.texto)
			if !terminado(r.t) {
				go func() {
					time.Sleep(3 * time.Second)
					fyne.Do(func() {
						if abierto {
							pedir()
						}
					})
				}()
			}
		})
	}
	if terminado(t) {
		cancelar.Disable()
	}

	cabecera := container.NewVBox(datos, progreso, avance, container.NewHBox(cancelar, resultados))
	contenido := container.NewBorder(cabecera, nil, nil, nil,
		container.NewGridWrap(fyne.NewSize(780, 320), salida))
	d = dialog.NewCustom(t.Nombre, "Cerrar", contenido, v.app.win)
	d.SetOnClosed(func() {
		abierto = false
		v.cargar()
	})
	d.Show()
	pedir()
}

// buscarResultados muestra «Abrir resultados» si el clúster dejó la carpeta
// <carpeta>/resultados-<id8> en el Home.
func (v *vistaTrabajos) buscarResultados(t bus.Trabajo, boton *widget.Button, d dialog.Dialog) {
	if t.RutaHome == "" || len(t.ID) < 8 || boton.Visible() {
		return
	}
	nombre := "resultados-" + t.ID[:8]
	enSegundoPlano(func(ctx context.Context) (bus.Listado, error) {
		return v.app.cli.Listar(ctx, t.RutaHome, bus.MiUnidad)
	}, func(l bus.Listado, err error) {
		if err != nil {
			return
		}
		for _, c := range l.Carpetas {
			if c.Nombre != nombre {
				continue
			}
			ruta := c.Ruta
			boton.OnTapped = func() {
				d.Hide()
				if v.app.abrirEnMiUnidad != nil {
					v.app.abrirEnMiUnidad(ruta)
				}
			}
			boton.Show()
			return
		}
	})
}

// lineasVisibles quita las líneas «PROGRESO: n»: son para la barra.
func lineasVisibles(texto string) []string {
	var lineas []string
	for _, l := range strings.Split(texto, "\n") {
		if l != "" && !strings.HasPrefix(l, "PROGRESO:") {
			lineas = append(lineas, l)
		}
	}
	return lineas
}

// resumenDeAvance arma «35 % · iteración 7 · Equipos: a, b» con lo que el
// programa y el planificador escriben en la salida.
func resumenDeAvance(t bus.Trabajo, texto string) string {
	partes := []string{itoa(t.Progreso.Int64()) + " %"}
	if t.Mensaje != "" {
		partes = append(partes, t.Mensaje)
	}
	iteracion := ""
	equipos := ""
	for _, l := range strings.Split(texto, "\n") {
		if campos := strings.Fields(l); len(campos) >= 2 && campos[0] == "iter" {
			if _, err := strconv.Atoi(campos[1]); err == nil {
				iteracion = campos[1]
			}
		}
		if strings.HasPrefix(l, "Nodos: ") {
			equipos = strings.TrimPrefix(l, "Nodos: ")
		}
	}
	if iteracion != "" {
		partes = append(partes, "iteración "+iteracion)
	}
	if equipos != "" {
		partes = append(partes, "Equipos: "+equipos)
	}
	return strings.Join(partes, " · ")
}

func (v *vistaTrabajos) nodos() {
	enSegundoPlano(v.app.cli.NodosDelCluster, func(ns []bus.NodoCluster, err error) {
		if err != nil {
			v.app.error(err)
			return
		}
		if len(ns) == 0 {
			dialog.ShowInformation("Nodos", "El clúster no reporta nodos.", v.app.win)
			return
		}
		texto := ""
		for _, n := range ns {
			disponible := "no disponible"
			if n.Disponible {
				disponible = "disponible"
			}
			texto += n.Host + " · " + itoa(n.Slots.Int64()) + " ranuras · " + disponible + "\n"
		}
		dialog.ShowInformation("Nodos del clúster", texto, v.app.win)
	})
}

func estadoLegible(t bus.Trabajo) string {
	nombres := map[string]string{
		"ENCOLADO":   "en cola",
		"EJECUTANDO": "ejecutando",
		"COMPLETADO": "completado",
		"FALLIDO":    "falló",
		"CANCELADO":  "cancelado",
	}
	e := nombres[t.Estado]
	if e == "" {
		e = t.Estado
	}
	if t.Estado == "EJECUTANDO" && t.Progreso > 0 {
		e += " · " + itoa(t.Progreso.Int64()) + "%"
	}
	return e
}

func detalleLargoDeTrabajo(t bus.Trabajo) string {
	texto := "Estado: " + estadoLegible(t) + "\n" +
		"Procesos: " + itoa(t.Procesos.Int64()) + "\n" +
		"Comando: " + t.Comando + "\n"
	if t.RutaHome != "" {
		texto += "Carpeta: " + t.RutaHome + "\n"
	}
	texto += "Enviado: " + fecha(t.CreadoEn)
	if t.DuracionSeg > 0 {
		texto += " · duró " + duracion(t.DuracionSeg.Int64())
	}
	if t.Mensaje != "" {
		texto += "\n" + t.Mensaje
	}
	return texto
}
