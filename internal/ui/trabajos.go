package ui

import (
	"context"
	"strconv"

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
	})
}

func (v *vistaTrabajos) enviar() {
	nombre := widget.NewEntry()
	comando := widget.NewEntry()
	comando.SetPlaceHolder("./mi-programa  (dentro de la carpeta del Home)")
	rutaHome := widget.NewEntry()
	rutaHome.SetPlaceHolder("/carpeta-del-trabajo")
	procesos := widget.NewEntry()
	procesos.SetText("2")

	dialog.ShowForm("Enviar trabajo MPI", "Enviar", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Nombre", nombre),
		widget.NewFormItem("Comando", comando),
		widget.NewFormItem("Carpeta en Mi unidad", rutaHome),
		widget.NewFormItem("Procesos", procesos),
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
}

func (v *vistaTrabajos) detalle(t bus.Trabajo) {
	salida := widget.NewMultiLineEntry()
	salida.Wrapping = fyne.TextWrapOff
	salida.SetText("Pidiendo la salida…")

	enSegundoPlano(func(ctx context.Context) (string, error) {
		return v.app.cli.SalidaDelTrabajo(ctx, t.ID)
	}, func(s string, err error) {
		if err != nil {
			salida.SetText("No se pudo leer la salida: " + err.Error())
			return
		}
		if s == "" {
			s = "(sin salida todavía)"
		}
		salida.SetText(s)
	})

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
	if t.Estado != "ENCOLADO" && t.Estado != "EJECUTANDO" {
		cancelar.Disable()
	}

	contenido := container.NewBorder(
		container.NewVBox(widget.NewLabel(detalleLargoDeTrabajo(t)), cancelar), nil, nil, nil,
		container.NewGridWrap(fyne.NewSize(780, 320), salida))
	d = dialog.NewCustom(t.Nombre, "Cerrar", contenido, v.app.win)
	d.Show()
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
