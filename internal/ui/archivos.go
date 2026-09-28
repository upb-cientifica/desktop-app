package ui

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/upb-cientifica/desktop-app/internal/bus"
)

// vistaArchivos dibuja el Home: Mi unidad y sus carpetas, Destacados o
// Papelera. Las tres se listan igual y solo cambian las acciones.
func (a *App) vistaArchivos(seccion bus.Seccion) armada {
	v := &vistaDeArchivos{app: a, seccion: seccion, ruta: "/"}
	if seccion == bus.MiUnidad {
		a.miUnidad = v
	}
	return armada{vista: v.construir(), refrescar: v.cargar, buscar: v.buscar, conTitulo: true}
}

// vistaCompartidos lista lo que otras cuentas compartieron conmigo, que llega
// como lista plana y con el propietario de cada archivo.
func (a *App) vistaCompartidos() armada {
	v := &vistaDeArchivos{app: a, compartidos: true}
	return armada{vista: v.construir(), refrescar: v.cargar, buscar: v.buscar, conTitulo: true}
}

type vistaDeArchivos struct {
	app         *App
	seccion     bus.Seccion
	compartidos bool

	ruta     string
	nodos    []bus.Nodo // lo que devolvió el servicio
	visibles []bus.Nodo // lo que queda tras el buscador y los chips
	consulta string
	tipo     string // "" todos, "imagen", "video"

	lista  *widget.List
	titulo *fyne.Container
	chips  []*pastilla
	estado *widget.Label
}

// Columnas de la lista, como la tabla de la web: icono, nombre, propietario,
// modificación, tamaño y el botón de acciones.
var anchosDeArchivos = columnas{anchos: []float32{24, 0, 150, 150, 90, 36}}

func (v *vistaDeArchivos) construir() fyne.CanvasObject {
	v.titulo = container.NewHBox()
	v.estado = widget.NewLabel("")
	v.estado.Importance = widget.LowImportance

	// Las filas se reciclan: cada pasada reescribe los textos y el botón.
	v.lista = widget.NewList(
		func() int { return len(v.visibles) },
		func() fyne.CanvasObject {
			nombre := widget.NewLabel("nombre")
			nombre.Truncation = fyne.TextTruncateEllipsis
			mas := widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)
			mas.Importance = widget.LowImportance
			return container.New(anchosDeArchivos,
				widget.NewIcon(iconoDocumento), nombre,
				secundaria(), secundaria(), secundaria(), mas)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.visibles) {
				return
			}
			n := v.visibles[i]
			celdas := o.(*fyne.Container).Objects
			celdas[0].(*widget.Icon).SetResource(iconoDe(n))
			nombre := n.Nombre
			if n.Destacado && v.seccion != bus.Destacados {
				nombre += "  ★"
			}
			celdas[1].(*widget.Label).SetText(nombre)
			celdas[2].(*widget.Label).SetText(v.propietario(n))
			celdas[3].(*widget.Label).SetText(fecha(n.ModificadoEn))
			tam := "—"
			if !n.EsCarpeta {
				tam = bus.Legible(n.TamanoBytes)
			}
			celdas[4].(*widget.Label).SetText(tam)
			celdas[5].(*widget.Button).OnTapped = func() { v.acciones(n) }
		},
	)
	v.lista.OnSelected = func(i widget.ListItemID) {
		v.lista.Unselect(i)
		if i < 0 || i >= len(v.visibles) {
			return
		}
		n := v.visibles[i]
		if n.EsCarpeta && v.seccion == bus.MiUnidad && !v.compartidos {
			v.ir(n.Ruta)
			return
		}
		v.acciones(n)
	}

	encabezado := container.New(anchosDeArchivos, widget.NewLabel(""),
		columna("Nombre"), columna("Propietario"), columna("Modificación"), columna("Tamaño"),
		widget.NewLabel(""))

	arriba := container.NewVBox(v.titulo, v.controles(),
		container.New(layout.NewCustomPaddedLayout(8, 0, 0, 0), encabezado), widget.NewSeparator())
	v.pintarTitulo()
	v.cargar()
	return container.NewBorder(arriba, v.estado, nil, nil, v.lista)
}

func secundaria() *widget.Label {
	l := widget.NewLabel("")
	l.Importance = widget.LowImportance
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

func columna(texto string) *widget.Label {
	return widget.NewLabelWithStyle(texto, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// controles son los chips de filtro y, a la derecha, lo propio de la sección.
func (v *vistaDeArchivos) controles() fyne.CanvasObject {
	opciones := []struct{ texto, tipo string }{
		{"Todos", ""}, {"Fotos", "imagen"}, {"Videos", "video"},
	}
	fila := container.NewHBox()
	for _, o := range opciones {
		o := o
		c := chip(o.texto, nil)
		c.alTocar = func(*fyne.PointEvent) {
			v.tipo = o.tipo
			for _, otro := range v.chips {
				otro.SetActivo(otro == c)
			}
			v.filtrar()
		}
		c.activo = o.tipo == ""
		v.chips = append(v.chips, c)
		fila.Add(c)
	}

	derecha := container.NewHBox()
	if v.seccion == bus.Papelera {
		vaciar := widget.NewButtonWithIcon("Vaciar papelera", theme.DeleteIcon(), v.vaciarPapelera)
		vaciar.Importance = widget.LowImportance
		derecha.Add(vaciar)
	}
	actualizar := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), v.cargar)
	actualizar.Importance = widget.LowImportance
	derecha.Add(actualizar)

	return container.New(layout.NewCustomPaddedLayout(4, 4, 0, 0),
		container.NewBorder(nil, nil, fila, derecha))
}

// pintarTitulo escribe el título grande: el nombre de la sección o, dentro de
// Mi unidad, la ruta de carpetas, donde cada tramo lleva de vuelta a ella.
func (v *vistaDeArchivos) pintarTitulo() {
	grande := func(t string) *widget.Label {
		l := widget.NewLabel(t)
		l.SizeName = tamTitulo
		return l
	}
	v.titulo.Objects = nil
	switch {
	case v.compartidos:
		v.titulo.Add(grande("Compartido conmigo"))
	case v.seccion == bus.Destacados:
		v.titulo.Add(grande("Destacados"))
	case v.seccion == bus.Papelera:
		v.titulo.Add(grande("Papelera"))
		nota := widget.NewLabel("Lo eliminado se conserva hasta vaciar la papelera")
		nota.Importance = widget.LowImportance
		v.titulo.Add(nota)
	case v.ruta == "/":
		v.titulo.Add(grande("Mi unidad"))
	default:
		atras := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { v.ir(bus.Padre(v.ruta)) })
		atras.Importance = widget.LowImportance
		v.titulo.Add(container.NewCenter(atras))

		tramos := strings.Split(strings.Trim(v.ruta, "/"), "/")
		v.titulo.Add(v.tramo("Mi unidad", "/"))
		for i, t := range tramos {
			v.titulo.Add(grande("›"))
			if i == len(tramos)-1 {
				v.titulo.Add(grande(t))
			} else {
				v.titulo.Add(v.tramo(t, "/"+strings.Join(tramos[:i+1], "/")))
			}
		}
	}
	v.titulo.Refresh()
}

// tramo es un pedazo de la ruta que se puede pulsar.
func (v *vistaDeArchivos) tramo(texto, ruta string) fyne.CanvasObject {
	p := nuevaPastilla(texto, nil, func(*fyne.PointEvent) { v.ir(ruta) })
	p.tam = theme.SizeForWidget(tamTitulo, p)
	p.colorTexto = colorTexto2
	p.izq, p.der, p.alto = 10, 10, 44
	return container.NewCenter(p)
}

// ---------- datos ----------

func (v *vistaDeArchivos) ir(ruta string) {
	v.ruta = ruta
	v.pintarTitulo()
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
		v.filtrar()
	})
}

// buscar lo llama el buscador de la barra de arriba.
func (v *vistaDeArchivos) buscar(texto string) {
	v.consulta = strings.ToLower(strings.TrimSpace(texto))
	v.filtrar()
}

// filtrar aplica el buscador y los chips sobre lo que ya llegó, sin volver a
// pedirlo al servidor.
func (v *vistaDeArchivos) filtrar() {
	v.visibles = v.visibles[:0]
	for _, n := range v.nodos {
		if v.consulta != "" && !strings.Contains(strings.ToLower(n.Nombre), v.consulta) {
			continue
		}
		if v.tipo != "" && (n.EsCarpeta || n.Tipo != v.tipo) {
			continue
		}
		v.visibles = append(v.visibles, n)
	}
	v.lista.Refresh()
	switch {
	case len(v.nodos) == 0:
		v.estado.SetText("Aquí no hay nada.")
	case len(v.visibles) == 0:
		v.estado.SetText("Nada coincide con el filtro.")
	default:
		v.estado.SetText(contar(v.visibles))
	}
}

// propietario dice «yo» para lo propio, como la web.
func (v *vistaDeArchivos) propietario(n bus.Nodo) string {
	u := v.app.ses.Usuario()
	if n.Propietario == "" || n.Propietario == u.ID || strings.HasPrefix(u.Correo, n.Propietario+"@") {
		return "yo"
	}
	return n.Propietario
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

// iconoDe usa los colores de la web: carpetas grises, fotos y videos en rojo
// y el resto de documentos en azul.
func iconoDe(n bus.Nodo) fyne.Resource {
	switch {
	case n.EsCarpeta:
		return theme.NewColoredResource(iconoCarpeta, colorTexto2)
	case n.Tipo == "imagen":
		return theme.NewColoredResource(iconoImagen, theme.ColorNameError)
	case n.Tipo == "video":
		return theme.NewColoredResource(iconoVideo, theme.ColorNameError)
	default:
		return theme.NewColoredResource(iconoDocumento, theme.ColorNamePrimary)
	}
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
