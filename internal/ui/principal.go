package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
	"github.com/upb-cientifica/desktop-app/internal/sincro"
)

// seccion es cada entrada del menú lateral. `servicio` es el código con el que
// el servicio viaja en el claim del token: si la cuenta no lo tiene, la
// sección se muestra deshabilitada en vez de fallar al abrirla. `grupo`
// separa, como en la web, los apartados del Home de los demás servicios.
type seccion struct {
	nombre    string
	icono     fyne.Resource
	servicio  string
	soloAdmin bool
	grupo     int
	abrir     func() armada
}

// armada es una sección ya construida. `buscar` filtra lo que muestra con el
// texto del buscador de arriba; si es nil, el buscador se apaga en esa
// sección. `conTitulo` avisa que la vista pinta su propio título, como la
// ruta de carpetas de Mi unidad.
type armada struct {
	vista     fyne.CanvasObject
	refrescar func()
	buscar    func(string)
	conTitulo bool
}

// simple adapta las vistas que no buscan ni titulan.
func simple(f func() (fyne.CanvasObject, func())) func() armada {
	return func() armada {
		v, r := f()
		return armada{vista: v, refrescar: r}
	}
}

func (a *App) secciones() []seccion {
	return []seccion{
		{nombre: "Mi unidad", icono: iconoMiUnidad, servicio: "shared_file",
			abrir: func() armada { return a.vistaArchivos(bus.MiUnidad) }},
		{nombre: "Compartido conmigo", icono: iconoGrupo, servicio: "shared_file",
			abrir: a.vistaCompartidos},
		{nombre: "Destacados", icono: iconoEstrella, servicio: "shared_file",
			abrir: func() armada { return a.vistaArchivos(bus.Destacados) }},
		{nombre: "Papelera", icono: iconoPapelera, servicio: "shared_file",
			abrir: func() armada { return a.vistaArchivos(bus.Papelera) }},
		{nombre: "Fotos", icono: iconoFotos, servicio: "photo_album", grupo: 1,
			abrir: a.vistaFotos},
		{nombre: "Videos", icono: iconoPelicula, servicio: "streaming", grupo: 1,
			abrir: simple(a.vistaVideos)},
		{nombre: "Sincronización", icono: iconoSincro, servicio: "file_sync", grupo: 1,
			abrir: simple(a.vistaSincronizacion)},
		{nombre: "Trabajos MPI", icono: iconoTerminal, servicio: "hpc", grupo: 1,
			abrir: simple(a.vistaTrabajos)},
		{nombre: "Monitoreo", icono: iconoMonitoreo, servicio: "monitoreo", grupo: 1,
			abrir: simple(a.vistaMonitoreo)},
		{nombre: "Administración", icono: iconoAdmin, servicio: "", soloAdmin: true, grupo: 1,
			abrir: simple(a.vistaAdministracion)},
	}
}

// anchoLateral es el del menú de la web.
const anchoLateral = 256

// mostrarPrincipal arma el marco, igual que la web: barra de arriba con el
// buscador, menú lateral con «Nuevo» y el área de la sección abierta.
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
	titulo := widget.NewLabel("")
	titulo.SizeName = tamTitulo
	cabecera := container.New(layout.NewCustomPaddedLayout(4, 0, 0, 0), titulo)

	buscador := widget.NewEntry()
	buscador.SetPlaceHolder("Buscar en Drive")
	var buscar func(string)
	buscador.OnChanged = func(t string) {
		if buscar != nil {
			buscar(t)
		}
	}

	// Cada sección se arma una sola vez y se guarda: volver a ella conserva la
	// carpeta donde ibas y no deja corriendo dos veces el refresco del
	// Monitoreo, que se actualiza solo. Pero al volver se recargan sus datos:
	// si no, lo que cambió desde otra sección —un archivo que se mandó a la
	// papelera desde Mi unidad— no aparecía hasta pulsar Actualizar.
	armadas := map[string]armada{}
	entradas := make([]*pastilla, len(secs))

	abrir := func(i int) {
		s := secs[i]
		for j, e := range entradas {
			e.SetActivo(j == i)
		}
		ya, existe := armadas[s.nombre]
		switch {
		case existe && ya.refrescar != nil:
			ya.refrescar()
		case !existe && a.tieneServicio(s.servicio):
			ya = s.abrir()
			armadas[s.nombre] = ya
		case !existe:
			ya.vista = sinPermiso(s.nombre)
			armadas[s.nombre] = ya
		}

		// Primero se limpia el buscador —eso quita el filtro de la sección que
		// se deja— y luego se le pasa a la nueva.
		buscador.SetText("")
		buscar = ya.buscar
		if buscar == nil {
			buscador.Disable()
		} else {
			buscador.Enable()
		}

		titulo.SetText(s.nombre)
		if ya.conTitulo {
			cabecera.Hide()
		} else {
			cabecera.Show()
		}
		area.Objects = []fyne.CanvasObject{ya.vista}
		area.Refresh()
	}
	indiceDe := func(nombre string) int {
		for i, s := range secs {
			if s.nombre == nombre {
				return i
			}
		}
		return -1
	}

	// Menú lateral
	nav := container.NewVBox()
	grupo := 0
	for i, s := range secs {
		if s.grupo != grupo {
			nav.Add(separadorLateral())
			grupo = s.grupo
		}
		i := i
		entradas[i] = elementoMenu(s.nombre, s.icono, func() { abrir(i) })
		nav.Add(entradas[i])
	}

	// «Nuevo» sube o crea siempre en Mi unidad, en la carpeta donde se esté.
	enMiUnidad := func(hacer func(*vistaDeArchivos)) func() {
		return func() {
			i := indiceDe("Mi unidad")
			if i < 0 || !a.tieneServicio("shared_file") {
				dialog.ShowInformation("Nuevo", "Tu cuenta no tiene habilitado Mi unidad.", a.win)
				return
			}
			abrir(i)
			if a.miUnidad != nil {
				hacer(a.miUnidad)
			}
		}
	}
	a.abrirEnMiUnidad = func(ruta string) {
		enMiUnidad(func(v *vistaDeArchivos) { v.ir(ruta) })()
	}
	menuNuevo := fyne.NewMenu("",
		fyne.NewMenuItemWithIcon("Subir archivo", theme.UploadIcon(),
			enMiUnidad(func(v *vistaDeArchivos) { v.subir() })),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon("Crear carpeta", theme.FolderNewIcon(),
			enMiUnidad(func(v *vistaDeArchivos) { v.nuevaCarpeta() })),
	)
	var nuevo *pastilla
	nuevo = nuevaPastilla("Nuevo", theme.ContentAddIcon(), func(*fyne.PointEvent) {
		widget.ShowPopUpMenuAtRelativePosition(menuNuevo, a.win.Canvas(),
			fyne.NewPos(0, nuevo.Size().Height+4), nuevo)
	})
	nuevo.alto, nuevo.radio, nuevo.der = 52, 16, 24
	nuevo.fondo, nuevo.fondoEncima = colorSuperficie2, colorSuperficie3
	nuevo.colorIcono, nuevo.negrita = theme.ColorNamePrimary, true

	lateral := container.New(anchoFijo{anchoLateral}, container.NewBorder(nil, nil, nil, widget.NewSeparator(),
		container.NewBorder(
			container.New(layout.NewCustomPaddedLayout(12, 8, 8, 0), container.NewHBox(nuevo)),
			a.almacenamiento(), nil, nil,
			container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, 0, 8, 12), nav)))))

	contenido := container.New(layout.NewCustomPaddedLayout(4, 8, 20, 20),
		container.NewBorder(cabecera, nil, nil, nil, area))

	// El menú de tres rayas esconde el lateral, y se recuerda para la próxima.
	cuerpo := container.NewStack()
	pintarCuerpo := func() {
		if a.fyne.Preferences().Bool("menuPlegado") {
			cuerpo.Objects = []fyne.CanvasObject{contenido}
		} else {
			cuerpo.Objects = []fyne.CanvasObject{container.NewBorder(nil, nil, lateral, nil, contenido)}
		}
		cuerpo.Refresh()
	}
	plegar := func() {
		p := a.fyne.Preferences()
		p.SetBool("menuPlegado", !p.Bool("menuPlegado"))
		pintarCuerpo()
	}
	pintarCuerpo()

	a.win.SetContent(container.NewBorder(a.barraSuperior(u, buscador, plegar), nil, nil, nil, cuerpo))

	// El horario de sincronización corre aunque la sección esté cerrada.
	a.guardarPrograma(sincro.CargarPrograma(a.cfg.DirDatos, defaultCarpetaSincro()))

	abrir(0)
}

// barraSuperior es la franja de arriba de la web: menú, logo, buscador,
// ayuda, ajustes y la cuenta.
func (a *App) barraSuperior(u bus.Usuario, buscador *widget.Entry, plegar func()) fyne.CanvasObject {
	menu := widget.NewButtonWithIcon("", theme.MenuIcon(), plegar)
	menu.Importance = widget.LowImportance

	logo := widget.NewLabel("Drive Upb")
	logo.SizeName = tamTitulo
	logo.Importance = widget.MediumImportance
	marca := container.NewHBox(menu,
		container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(34),
			widget.NewIcon(theme.NewColoredResource(iconoNube, theme.ColorNamePrimary)))),
		logo)
	// La marca ocupa lo mismo que el menú lateral, para que el buscador
	// arranque donde empieza el contenido, como en la web.
	izquierda := container.New(anchoFijo{anchoLateral - 12}, marca)

	pildora := container.NewStack(nuevoFondo(colorSuperficie3, 24),
		container.New(layout.NewCustomPaddedLayout(4, 4, 12, 16), container.NewBorder(nil, nil,
			widget.NewIcon(theme.NewColoredResource(theme.SearchIcon(), colorTexto2)), nil,
			container.NewThemeOverride(buscador, temaSinMarco{}))))
	busqueda := container.New(hastaAncho{720}, pildora)

	ayuda := widget.NewButtonWithIcon("", theme.QuestionIcon(), a.mostrarAyuda)
	ayuda.Importance = widget.LowImportance
	var ajustes *widget.Button
	ajustes = widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		widget.ShowPopUpMenuAtRelativePosition(a.menuDeTema(), a.win.Canvas(),
			fyne.NewPos(-160, ajustes.Size().Height), ajustes)
	})
	ajustes.Importance = widget.LowImportance
	derecha := container.NewHBox(ayuda, ajustes,
		container.NewCenter(nuevoAvatar(u.Nombre, func() { a.menuCuenta(u) })))

	barra := container.NewBorder(nil, nil, izquierda, derecha, busqueda)
	return container.NewVBox(container.New(layout.NewCustomPaddedLayout(8, 8, 8, 12), barra),
		widget.NewSeparator())
}

// menuDeTema es el menú de Ajustes: el mismo interruptor de tres estados que
// en la web.
func (a *App) menuDeTema() *fyne.Menu {
	actual := a.preferenciaDeTema()
	opcion := func(texto string, p Preferencia) *fyne.MenuItem {
		it := fyne.NewMenuItem(texto, func() { a.aplicarTema(p) })
		it.Checked = actual == p
		return it
	}
	return fyne.NewMenu("",
		opcion("Tema del sistema", TemaSistema),
		opcion("Tema claro", TemaClaro),
		opcion("Tema oscuro", TemaOscuro),
	)
}

func (a *App) mostrarAyuda() {
	dialog.ShowInformation("Ayuda",
		"Nuevo sube un archivo o crea una carpeta en Mi unidad.\n"+
			"Los tres puntos de cada fila muestran lo que puedes hacer con ese elemento.\n"+
			"El buscador filtra la lista de la sección abierta.\n"+
			"En Ajustes (la tuerca) eliges tema claro, oscuro o el del sistema.\n"+
			"Sincronización mantiene una carpeta de tu equipo al día con el servidor.",
		a.win)
}

func separadorLateral() fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(6, 6, 12, 4), widget.NewSeparator())
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

// almacenamiento es el pie del menú lateral: cuánto del Home está ocupado.
func (a *App) almacenamiento() fyne.CanvasObject {
	texto := widget.NewLabel("Cargando almacenamiento…")
	texto.Importance = widget.LowImportance
	barra := nuevaBarraFina()

	refrescar := func() {
		enSegundoPlano(a.cli.UsoDelHome, func(u bus.Uso, err error) {
			if err != nil {
				texto.SetText("Almacenamiento no disponible")
				return
			}
			texto.SetText(bus.Legible(u.UsadoBytes) + " de " + bus.Legible(u.CuotaBytes) + " usados")
			if u.CuotaBytes > 0 {
				barra.SetValor(float64(u.UsadoBytes) / float64(u.CuotaBytes))
			}
		})
	}
	refrescar()
	a.refrescarCuota = refrescar

	titulo := elementoMenu("Almacenamiento", iconoNube, func() {})
	titulo.fondoEncima = ""
	return container.New(layout.NewCustomPaddedLayout(0, 12, 8, 20), container.NewVBox(
		separadorLateral(), titulo,
		container.New(layout.NewCustomPaddedLayout(0, 0, 16, 0), barra),
		container.New(layout.NewCustomPaddedLayout(0, 0, 8, 0), texto)))
}

func (a *App) menuCuenta(u bus.Usuario) {
	nombres := map[Preferencia]string{
		TemaSistema: "Como el sistema",
		TemaClaro:   "Claro",
		TemaOscuro:  "Oscuro",
	}
	tema := widget.NewRadioGroup(
		[]string{nombres[TemaSistema], nombres[TemaClaro], nombres[TemaOscuro]},
		func(elegido string) {
			for p, n := range nombres {
				if n == elegido {
					a.aplicarTema(p)
				}
			}
		})
	tema.Horizontal = true
	tema.SetSelected(nombres[a.preferenciaDeTema()])

	contenido := container.NewVBox(
		widget.NewLabel(u.Nombre+"\n"+u.Correo+"\nRol: "+u.Rol),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Tema", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		tema,
	)
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
