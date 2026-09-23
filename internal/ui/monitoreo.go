package ui

import (
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaMonitoreo enseña cómo va la máquina y qué servicios responden. Se
// refresca sola cada diez segundos mientras la sección está abierta.
type vistaMonitoreo struct {
	app *App

	cpu, memoria, disco *widget.ProgressBar
	textos              map[string]*widget.Label
	servicios           *widget.List
	listaServicios      []bus.ServicioVigilado
	estado              *widget.Label
	detener             chan struct{}
}

func (a *App) vistaMonitoreo() (fyne.CanvasObject, func()) {
	v := &vistaMonitoreo{app: a, textos: map[string]*widget.Label{}, detener: make(chan struct{})}

	v.cpu, v.memoria, v.disco = widget.NewProgressBar(), widget.NewProgressBar(), widget.NewProgressBar()
	for _, b := range []*widget.ProgressBar{v.cpu, v.memoria, v.disco} {
		b.Min, b.Max = 0, 100
	}
	v.estado = widget.NewLabel("Consultando el monitoreo…")

	v.servicios = widget.NewList(
		func() int { return len(v.listaServicios) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil,
				widget.NewIcon(theme.RadioButtonIcon()),
				widget.NewLabel("latencia"),
				widget.NewLabel("servicio"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.listaServicios) {
				return
			}
			s := v.listaServicios[i]
			fila := o.(*fyne.Container)
			fila.Objects[0].(*widget.Label).SetText(s.Nombre)
			icono := theme.RadioButtonIcon()
			if s.Estado == "disponible" {
				icono = theme.ConfirmIcon()
			} else {
				icono = theme.ErrorIcon()
			}
			fila.Objects[1].(*widget.Icon).SetResource(icono)
			fila.Objects[2].(*widget.Label).SetText(latencia(s))
		},
	)

	medidor := func(nombre string, barra *widget.ProgressBar) fyne.CanvasObject {
		etiqueta := widget.NewLabel("—")
		v.textos[nombre] = etiqueta
		return container.NewBorder(nil, nil, widget.NewLabel(nombre), etiqueta, barra)
	}

	arriba := container.NewVBox(
		widget.NewLabelWithStyle("Máquina", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		medidor("CPU", v.cpu),
		medidor("Memoria", v.memoria),
		medidor("Disco", v.disco),
		v.estado,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Servicios", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	v.refrescar()
	v.repetirCada(10 * time.Second)

	return container.NewBorder(arriba, nil, nil, nil, v.servicios), v.refrescar
}

func (v *vistaMonitoreo) repetirCada(cada time.Duration) {
	go func() {
		t := time.NewTicker(cada)
		defer t.Stop()
		for {
			select {
			case <-v.detener:
				return
			case <-t.C:
				fyne.Do(v.refrescar)
			}
		}
	}()
}

func (v *vistaMonitoreo) refrescar() {
	enSegundoPlano(v.app.cli.HostMonitoreado, func(h bus.Host, err error) {
		if err != nil {
			v.estado.SetText("El monitoreo no responde: " + err.Error())
			return
		}
		v.cpu.SetValue(h.CPUPct)
		v.memoria.SetValue(h.MemoriaPct)
		v.disco.SetValue(h.DiscoPct)
		v.textos["CPU"].SetText(pct(h.CPUPct))
		v.textos["Memoria"].SetText(pct(h.MemoriaPct))
		v.textos["Disco"].SetText(pct(h.DiscoPct))
		texto := "Actualizado a las " + time.Now().Format("15:04:05")
		if h.Uptime != "" {
			texto += " · encendida desde hace " + h.Uptime
		}
		v.estado.SetText(texto)
	})

	enSegundoPlano(v.app.cli.ServiciosVigilados, func(ss []bus.ServicioVigilado, err error) {
		if err != nil {
			return
		}
		v.listaServicios = ss
		v.servicios.Refresh()
	})
}

func pct(v float64) string {
	if v < 0 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + "%"
}

func latencia(s bus.ServicioVigilado) string {
	if s.Estado != "disponible" {
		return "no responde"
	}
	return itoa(s.LatenciaMs.Int64()) + " ms"
}

func (v *vistaMonitoreo) parar() {
	select {
	case <-v.detener:
	default:
		close(v.detener)
	}
}
