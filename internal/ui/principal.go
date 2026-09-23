package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/sincro"
)

// seccion es cada entrada del menú lateral. `servicio` es el código con el que
// el servicio viaja en el claim del token: si la cuenta no lo tiene, la
// sección se muestra deshabilitada en vez de fallar al abrirla.
type seccion struct {
	nombre    string
	icono     fyne.Resource
	servicio  string
	soloAdmin bool
	abrir     func() fyne.CanvasObject
}

func (a *App) secciones() []seccion {
	return []seccion{
		{nombre: "Mi unidad", icono: theme.StorageIcon(), servicio: "shared_file",
			abrir: func() fyne.CanvasObject { return a.vistaArchivos(bus.MiUnidad) }},
		{nombre: "Compartido conmigo", icono: theme.AccountIcon(), servicio: "shared_file",
			abrir: func() fyne.CanvasObject { return a.vistaCompartidos() }},
		{nombre: "Destacados", icono: theme.ConfirmIcon(), servicio: "shared_file",
			abrir: func() fyne.CanvasObject { return a.vistaArchivos(bus.Destacados) }},
		{nombre: "Papelera", icono: theme.DeleteIcon(), servicio: "shared_file",
			abrir: func() fyne.CanvasObject { return a.vistaArchivos(bus.Papelera) }},
		{nombre: "Sincronización", icono: theme.ViewRefreshIcon(), servicio: "file_sync",
			abrir: func() fyne.CanvasObject { return a.vistaSincronizacion() }},
		{nombre: "Fotos", icono: theme.MediaPhotoIcon(), servicio: "photo_album",
			abrir: func() fyne.CanvasObject { return a.vistaFotos() }},
		{nombre: "Videos", icono: theme.MediaVideoIcon(), servicio: "streaming",
			abrir: func() fyne.CanvasObject { return a.vistaVideos() }},
		{nombre: "Trabajos MPI", icono: theme.ComputerIcon(), servicio: "hpc",
			abrir: func() fyne.CanvasObject { return a.vistaTrabajos() }},
		{nombre: "Monitoreo", icono: theme.InfoIcon(), servicio: "monitoreo",
			abrir: func() fyne.CanvasObject { return a.vistaMonitoreo() }},
		{nombre: "Administración", icono: theme.SettingsIcon(), servicio: "", soloAdmin: true,
			abrir: func() fyne.CanvasObject { return a.vistaAdministracion() }},
	}
}

// mostrarPrincipal arma el marco: menú lateral, encabezado y el área de la
// sección abierta.
func (a *App) mostrarPrincipal() {
	u := a.ses.Usuario()
	secs := []seccion{}
	for _, s := range a.secciones() {
		if s.soloAdmin && !u.EsAdmin() {
			continue
		}
		secs = append(secs, s)
	}

	area := container.NewStack()
	titulo := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	menu := widget.NewList(
		func() int { return len(secs) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabel("plantilla"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			fila := o.(*fyne.Container)
			fila.Objects[0].(*widget.Icon).SetResource(secs[i].icono)
			etiqueta := fila.Objects[1].(*widget.Label)
			etiqueta.SetText(secs[i].nombre)
		},
	)

	// Cada sección se arma una sola vez y se guarda: volver a ella conserva la
	// carpeta donde ibas y no deja corriendo dos veces el refresco del
	// Monitoreo, que se actualiza solo.
	armadas := map[string]fyne.CanvasObject{}

	menu.OnSelected = func(i widget.ListItemID) {
		s := secs[i]
		titulo.SetText(s.nombre)
		vista, lista := armadas[s.nombre]
		if !lista {
			if a.tieneServicio(s.servicio) {
				vista = s.abrir()
			} else {
				vista = sinPermiso(s.nombre)
			}
			armadas[s.nombre] = vista
		}
		area.Objects = []fyne.CanvasObject{vista}
		area.Refresh()
	}

	cuenta := widget.NewButtonWithIcon(u.Nombre, theme.AccountIcon(), func() { a.menuCuenta(u) })
	cuenta.Importance = widget.LowImportance
	encabezado := container.NewBorder(nil, nil, titulo, cuenta)

	lateral := container.NewBorder(nil, a.barraCuota(), nil, nil, menu)
	division := container.NewHSplit(lateral, container.NewBorder(
		container.NewPadded(encabezado), nil, nil, nil, area))
	division.SetOffset(0.22)

	// El horario de sincronización corre aunque la sección esté cerrada.
	a.guardarPrograma(sincro.CargarPrograma(a.cfg.DirDatos, defaultCarpetaSincro()))

	a.win.SetContent(division)
	menu.Select(0)
}

// tieneServicio consulta el claim del token: el mismo permiso que aplica el bus.
func (a *App) tieneServicio(codigo string) bool {
	if codigo == "" {
		return true
	}
	u := a.ses.Usuario()
	return u.EsAdmin() || u.Servicios.Contiene(codigo)
}

func sinPermiso(nombre string) fyne.CanvasObject {
	etiqueta := widget.NewLabel("Tu cuenta no tiene habilitado el servicio de " + nombre +
		".\nPide a un administrador que te lo asigne.")
	etiqueta.Alignment = fyne.TextAlignCenter
	return container.NewCenter(etiqueta)
}

// barraCuota muestra cuánto del Home está ocupado, bajo el menú.
func (a *App) barraCuota() fyne.CanvasObject {
	texto := widget.NewLabel("Calculando…")
	barra := widget.NewProgressBar()
	barra.Min, barra.Max = 0, 100

	refrescar := func() {
		enSegundoPlano(a.cli.UsoDelHome, func(u bus.Uso, err error) {
			if err != nil {
				texto.SetText("Cuota no disponible")
				return
			}
			texto.SetText(bus.Legible(u.UsadoBytes) + " de " + bus.Legible(u.CuotaBytes))
			barra.SetValue(float64(u.Porcentaje))
		})
	}
	refrescar()
	a.refrescarCuota = refrescar

	return container.NewPadded(container.NewVBox(barra, texto))
}

func (a *App) menuCuenta(u bus.Usuario) {
	contenido := widget.NewLabel(u.Nombre + "\n" + u.Correo + "\nRol: " + u.Rol)
	d := dialog.NewCustomConfirm("Tu cuenta", "Cerrar sesión", "Volver", contenido, func(salir bool) {
		if !salir {
			return
		}
		go func() {
			ctx, cancelar := bus.ConPlazo(context.Background())
			defer cancelar()
			a.ses.Salir(ctx)
			fyne.Do(a.mostrarEntrada)
		}()
	}, a.win)
	d.Show()
}
