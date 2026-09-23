package ui

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/sincro"
)

// vistaSincronizacion es el requisito §3.3: una carpeta del equipo que se
// mantiene igual en el servidor, a mano o a una hora fija, con el historial de
// versiones y los conflictos a la vista.
//
// Es la única sección que no pasa por el bus: habla gRPC con el servidor de
// File Sync.
type vistaSincronizacion struct {
	app      *App
	programa sincro.Programa

	estadoTxt       *widget.Label
	carpetaTxt      *widget.Label
	bitacora        *widget.Entry
	conflictos      *widget.List
	listaConflictos []sincro.Conflicto
	corriendo       bool
}

func (a *App) vistaSincronizacion() (fyne.CanvasObject, func()) {
	v := &vistaSincronizacion{
		app:      a,
		programa: sincro.CargarPrograma(a.cfg.DirDatos, defaultCarpetaSincro()),
	}
	return v.construir(), v.refrescarEstado
}

func (v *vistaSincronizacion) construir() fyne.CanvasObject {
	v.carpetaTxt = widget.NewLabel(v.programa.Carpeta)
	v.estadoTxt = widget.NewLabel("Consultando el servidor…")
	v.bitacora = widget.NewMultiLineEntry()
	v.bitacora.Wrapping = fyne.TextWrapWord
	v.bitacora.SetPlaceHolder("Aquí sale lo que pasa en cada sincronización.")

	elegir := widget.NewButtonWithIcon("Cambiar carpeta", theme.FolderOpenIcon(), v.elegirCarpeta)
	registrar := widget.NewButtonWithIcon("Registrar este equipo", theme.ConfirmIcon(), v.registrar)
	sincronizar := widget.NewButtonWithIcon("Sincronizar ahora", theme.ViewRefreshIcon(), func() { v.sincronizar(true) })
	sincronizar.Importance = widget.HighImportance
	traer := widget.NewButtonWithIcon("Solo traer del servidor", theme.DownloadIcon(), func() { v.sincronizar(false) })
	abrir := widget.NewButtonWithIcon("Abrir carpeta", theme.FolderIcon(), v.abrirCarpeta)

	v.conflictos = widget.NewList(
		func() int { return len(v.listaConflictos) },
		func() fyne.CanvasObject { return widget.NewLabel("plantilla") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c := v.listaConflictos[i]
			o.(*widget.Label).SetText("#" + strconv.FormatInt(c.ID, 10) + "  " + c.Ruta +
				"  (servidor v" + strconv.FormatInt(c.VersionServidor, 10) + ")")
		},
	)
	v.conflictos.OnSelected = func(i widget.ListItemID) {
		v.conflictos.Unselect(i)
		if i >= 0 && i < len(v.listaConflictos) {
			v.resolver(v.listaConflictos[i])
		}
	}

	arriba := container.NewVBox(
		widget.NewLabelWithStyle("Carpeta sincronizada", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		v.carpetaTxt,
		container.NewHBox(elegir, abrir, registrar),
		widget.NewSeparator(),
		v.cuadroPrograma(),
		widget.NewSeparator(),
		container.NewHBox(sincronizar, traer),
		v.estadoTxt,
		widget.NewLabelWithStyle("Conflictos pendientes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	// Los conflictos ocupan una franja fija y la bitácora el resto.
	panelConflictos := container.NewGridWrap(fyne.NewSize(900, 120), v.conflictos)

	v.refrescarEstado()
	return container.NewBorder(arriba, nil, nil, nil,
		container.NewVSplit(panelConflictos, v.bitacora))
}

// cuadroPrograma es la parte "programable por horario" del requisito.
func (v *vistaSincronizacion) cuadroPrograma() fyne.CanvasObject {
	modos := map[string]sincro.Modo{
		"Solo a mano":               sincro.Manual,
		"Cada tantos minutos":       sincro.Intervalo,
		"Todos los días a una hora": sincro.Diario,
	}
	etiquetaDe := func(m sincro.Modo) string {
		for k, val := range modos {
			if val == m {
				return k
			}
		}
		return "Solo a mano"
	}

	minutos := widget.NewEntry()
	minutos.SetText(strconv.Itoa(v.programa.IntervaloMin))
	hora := widget.NewEntry()
	hora.SetText(v.programa.Hora)
	borrados := widget.NewCheck("Propagar también los borrados", nil)
	borrados.SetChecked(v.programa.PropagarBorrados)
	resumen := widget.NewLabel(v.programa.Descripcion())

	filaMinutos := container.NewHBox(widget.NewLabel("Cada"), minutos, widget.NewLabel("minutos"))
	filaHora := container.NewHBox(widget.NewLabel("A las"), hora, widget.NewLabel("(HH:MM)"))

	ajustarVisibles := func(m sincro.Modo) {
		filaMinutos.Hidden = m != sincro.Intervalo
		filaHora.Hidden = m != sincro.Diario
		filaMinutos.Refresh()
		filaHora.Refresh()
	}

	var modo *widget.Select
	guardar := func() {
		v.programa.Modo = modos[modo.Selected]
		if n, err := strconv.Atoi(minutos.Text); err == nil && n > 0 {
			v.programa.IntervaloMin = n
		}
		v.programa.Hora = hora.Text
		v.programa.PropagarBorrados = borrados.Checked
		resumen.SetText(v.programa.Descripcion())
		ajustarVisibles(v.programa.Modo)
		v.app.guardarPrograma(v.programa)
	}

	modo = widget.NewSelect(clavesOrdenadas(modos), func(string) { guardar() })
	modo.SetSelected(etiquetaDe(v.programa.Modo))
	minutos.OnChanged = func(string) { guardar() }
	hora.OnChanged = func(string) { guardar() }
	borrados.OnChanged = func(bool) { guardar() }
	ajustarVisibles(v.programa.Modo)

	return container.NewVBox(
		widget.NewLabelWithStyle("Cuándo sincronizar", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		modo, filaMinutos, filaHora, borrados, resumen,
	)
}

func clavesOrdenadas(m map[string]sincro.Modo) []string {
	// Orden fijo: de menos a más automático.
	return []string{"Solo a mano", "Cada tantos minutos", "Todos los días a una hora"}
}

// ---------- acciones ----------

func (v *vistaSincronizacion) elegirCarpeta() {
	dialog.ShowFolderOpen(func(lista fyne.ListableURI, err error) {
		if err != nil || lista == nil {
			return
		}
		v.programa.Carpeta = lista.Path()
		v.carpetaTxt.SetText(v.programa.Carpeta)
		v.app.guardarPrograma(v.programa)
		v.refrescarEstado()
	}, v.app.win)
}

func (v *vistaSincronizacion) abrirCarpeta() {
	if err := os.MkdirAll(v.programa.Carpeta, 0o755); err != nil {
		v.app.error(err)
		return
	}
	v.app.abrirEnElSistema(v.programa.Carpeta)
}

func (v *vistaSincronizacion) registrar() {
	nombre := widget.NewEntry()
	nombre.SetText(nombreDeEsteEquipo())
	dialog.ShowForm("Registrar este equipo", "Registrar", "Cancelar",
		[]*widget.FormItem{widget.NewFormItem("Nombre", nombre)}, func(ok bool) {
			if !ok || nombre.Text == "" {
				return
			}
			if err := os.MkdirAll(v.programa.Carpeta, 0o755); err != nil {
				v.app.error(err)
				return
			}
			carpeta := v.programa.Carpeta
			enSegundoPlano(func(ctx context.Context) (string, error) {
				cli, err := v.app.sincro()
				if err != nil {
					return "", err
				}
				id, err := cli.RegistrarDispositivo(ctx, nombre.Text, plataforma(), carpeta)
				if err != nil {
					return "", err
				}
				st := sincro.CargarEstado(carpeta)
				st.DispositivoID = id
				st.Endpoint = v.app.cfg.SincroAddr
				return id, st.Guardar()
			}, func(id string, err error) {
				if err != nil {
					v.app.error(err)
					return
				}
				v.escribir("Equipo registrado con el identificador " + id)
				v.refrescarEstado()
			})
		}, v.app.win)
}

// sincronizar hace una pasada. subir=false solo trae lo del servidor.
func (v *vistaSincronizacion) sincronizar(subir bool) {
	if v.corriendo {
		return
	}
	carpeta := v.programa.Carpeta
	st := sincro.CargarEstado(carpeta)
	if !st.Registrado() {
		dialog.ShowInformation("Falta registrar",
			"Registra este equipo antes de sincronizar.", v.app.win)
		return
	}

	v.corriendo = true
	v.estadoTxt.SetText("Sincronizando…")
	propagarBorrados := v.programa.PropagarBorrados

	go func() {
		// Una carpeta grande puede tardar: plazo amplio y propio.
		ctx, cancelar := context.WithTimeout(context.Background(), time.Hour)
		defer cancelar()

		cli, err := v.app.sincro()
		if err != nil {
			fyne.Do(func() {
				v.corriendo = false
				v.estadoTxt.SetText("No se pudo conectar con File Sync: " + err.Error())
			})
			return
		}
		lineas := make([]string, 0, 32)
		r := cli.Sincronizar(ctx, st, carpeta, sincro.Opciones{
			Subir:  subir,
			Borrar: subir && propagarBorrados,
			Aviso:  func(l string) { lineas = append(lineas, l) },
		})
		errGuardar := st.Guardar()

		fyne.Do(func() {
			v.corriendo = false
			for _, l := range lineas {
				v.escribir(l)
			}
			for _, f := range r.Fallos {
				v.escribir("✗ " + f)
			}
			if errGuardar != nil {
				v.escribir("✗ no se pudo guardar el estado local: " + errGuardar.Error())
			}
			v.escribir(time.Now().Format("15:04:05") + " · " + r.String())
			v.app.marcarSincronizacion()
			v.refrescarEstado()
		})
	}()
}

func (v *vistaSincronizacion) resolver(c sincro.Conflicto) {
	opciones := widget.NewRadioGroup([]string{
		"Quedarme con la versión del servidor",
		"Que gane mi copia de este equipo",
		"Conservar las dos (la mía se guarda aparte)",
	}, nil)
	opciones.SetSelected("Quedarme con la versión del servidor")

	contenido := container.NewVBox(
		widget.NewLabel("Archivo: "+c.Ruta),
		widget.NewLabel("El servidor va en la versión "+strconv.FormatInt(c.VersionServidor, 10)+"."),
		opciones,
	)
	dialog.ShowCustomConfirm("Conflicto #"+strconv.FormatInt(c.ID, 10), "Resolver", "Cancelar",
		contenido, func(ok bool) {
			if !ok {
				return
			}
			estrategia := sincro.MantenerServidor
			switch opciones.Selected {
			case "Que gane mi copia de este equipo":
				estrategia = sincro.MantenerCliente
			case "Conservar las dos (la mía se guarda aparte)":
				estrategia = sincro.MantenerAmbos
			}
			carpeta := v.programa.Carpeta
			enSegundoPlano(func(ctx context.Context) (string, error) {
				cli, err := v.app.sincro()
				if err != nil {
					return "", err
				}
				ruta, copia, err := cli.Resolver(ctx, c.ID, estrategia)
				if err != nil {
					return "", err
				}
				if ruta == "" {
					ruta = c.Ruta
				}
				// Se olvida la ruta para que la siguiente pasada aplique el
				// resultado de la resolución en vez de verla como cambio local.
				st := sincro.CargarEstado(carpeta)
				st.Olvidar(ruta)
				_ = st.Guardar()
				return copia, nil
			}, func(copia string, err error) {
				if err != nil {
					v.app.error(err)
					return
				}
				v.escribir("Conflicto #" + strconv.FormatInt(c.ID, 10) + " resuelto.")
				if copia != "" {
					v.escribir("Tu copia quedó guardada como: " + copia)
				}
				v.sincronizar(false)
			})
		}, v.app.win)
}

// ---------- estado ----------

func (v *vistaSincronizacion) refrescarEstado() {
	carpeta := v.programa.Carpeta
	enSegundoPlano(func(ctx context.Context) (estadoSincro, error) {
		cli, err := v.app.sincro()
		if err != nil {
			return estadoSincro{}, err
		}
		e, err := cli.Estado(ctx)
		if err != nil {
			return estadoSincro{}, err
		}
		cs, err := cli.Conflictos(ctx, false)
		if err != nil {
			return estadoSincro{}, err
		}
		return estadoSincro{servidor: e, conflictos: cs, registrado: sincro.CargarEstado(carpeta).Registrado()}, nil
	}, func(e estadoSincro, err error) {
		if err != nil {
			v.estadoTxt.SetText("File Sync no responde: " + err.Error())
			return
		}
		v.listaConflictos = e.conflictos
		v.conflictos.Refresh()

		texto := "En el servidor: " + strconv.FormatInt(e.servidor.Archivos, 10) + " archivos · " +
			legible(e.servidor.Bytes) + " · última sincronización: " + oGuion(e.servidor.UltimaSync)
		if !e.registrado {
			texto = "Este equipo todavía no está registrado. " + texto
		}
		if len(e.conflictos) == 0 {
			texto += " · sin conflictos"
		}
		v.estadoTxt.SetText(texto)
	})
}

type estadoSincro struct {
	servidor   sincro.Estado
	conflictos []sincro.Conflicto
	registrado bool
}

func (v *vistaSincronizacion) escribir(linea string) {
	v.bitacora.SetText(v.bitacora.Text + linea + "\n")
	v.bitacora.CursorRow = len(v.bitacora.Text)
}

// ---------- utilidades ----------

func nombreDeEsteEquipo() string {
	if n, err := os.Hostname(); err == nil && n != "" {
		return n
	}
	return "Equipo de escritorio"
}

func plataforma() string {
	switch runtime.GOOS {
	case "darwin":
		return "escritorio-macos"
	case "windows":
		return "escritorio-windows"
	default:
		return "escritorio-linux"
	}
}

func oGuion(s string) string {
	if s == "" {
		return "nunca"
	}
	return fecha(s)
}
