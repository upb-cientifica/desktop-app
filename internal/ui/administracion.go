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

const gb = 1024 * 1024 * 1024

// vistaAdministracion gestiona las cuentas del directorio. Todo pasa por el
// Servicio de Usuarios, que es SOAP: el bus arma el sobre y devuelve JSON.
// El propio servicio exige rol admin, así que aquí no se decide nada de eso.
type vistaAdministracion struct {
	app       *App
	usuarios  []bus.Usuario
	catalogos bus.Catalogos
	lista     *widget.List
	estado    *widget.Label
}

func (a *App) vistaAdministracion() (fyne.CanvasObject, func()) {
	v := &vistaAdministracion{app: a}
	v.estado = widget.NewLabel("Cargando el directorio…")

	v.lista = widget.NewList(
		func() int { return len(v.usuarios) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil,
				widget.NewIcon(theme.AccountIcon()),
				container.NewHBox(widget.NewLabel("estado"),
					widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), nil)),
				widget.NewLabel("nombre"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.usuarios) {
				return
			}
			u := v.usuarios[i]
			fila := o.(*fyne.Container)
			fila.Objects[0].(*widget.Label).SetText(u.Nombre + "  ·  " + u.Correo)
			derecha := fila.Objects[2].(*fyne.Container)
			derecha.Objects[0].(*widget.Label).SetText(resumenDeCuenta(u))
			derecha.Objects[1].(*widget.Button).OnTapped = func() { v.acciones(u) }
		},
	)
	v.lista.OnSelected = func(i widget.ListItemID) {
		v.lista.Unselect(i)
		if i >= 0 && i < len(v.usuarios) {
			v.acciones(v.usuarios[i])
		}
	}

	barra := container.NewHBox(
		widget.NewButtonWithIcon("Dar de alta", theme.ContentAddIcon(), v.crear),
		widget.NewButtonWithIcon("Sesiones abiertas", theme.HistoryIcon(), v.sesiones),
		widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), v.cargar),
	)

	v.cargarCatalogos()
	v.cargar()
	return container.NewBorder(container.NewVBox(barra, v.estado), nil, nil, nil, v.lista), v.cargar
}

func (v *vistaAdministracion) cargar() {
	v.estado.SetText("Cargando el directorio…")
	enSegundoPlano(v.app.cli.ListarUsuarios, func(us []bus.Usuario, err error) {
		if err != nil {
			v.estado.SetText("No se pudo cargar: " + err.Error())
			return
		}
		v.usuarios = us
		v.lista.Refresh()
		v.estado.SetText(itoa(int64(len(us))) + " cuentas")
	})
}

func (v *vistaAdministracion) cargarCatalogos() {
	enSegundoPlano(v.app.cli.Catalogos, func(c bus.Catalogos, err error) {
		if err == nil {
			v.catalogos = c
		}
	})
}

// crear da de alta una cuenta. Los campos van dentro de `datos`, que es como
// los agrupa el WSDL, y la cuenta nace con todo el catálogo de servicios: sin
// ellos entraría pero no podría abrir nada, porque el permiso viaja en el token.
func (v *vistaAdministracion) crear() {
	nombre := widget.NewEntry()
	correo := widget.NewEntry()
	correo.SetPlaceHolder("cuenta@" + bus.DominioCorreo)
	clave := widget.NewPasswordEntry()
	clave.SetPlaceHolder("Al menos 10 caracteres")
	cuota := widget.NewEntry()
	cuota.SetText("1")

	rol := widget.NewSelect(nil, nil)
	for _, r := range v.catalogos.Roles {
		rol.Options = append(rol.Options, r.Codigo)
	}
	if len(rol.Options) == 0 {
		rol.Options = []string{"investigador", "admin"}
	}
	rol.SetSelected(rol.Options[0])

	grupo := widget.NewSelect([]string{"Sin grupo"}, nil)
	for _, g := range v.catalogos.Grupos {
		grupo.Options = append(grupo.Options, g.Nombre)
	}
	grupo.SetSelected("Sin grupo")

	dialog.ShowForm("Dar de alta una cuenta", "Crear", "Cancelar", []*widget.FormItem{
		widget.NewFormItem("Nombre", nombre),
		widget.NewFormItem("Correo", correo),
		widget.NewFormItem("Contraseña", clave),
		widget.NewFormItem("Rol", rol),
		widget.NewFormItem("Grupo", grupo),
		widget.NewFormItem("Cuota (GB)", cuota),
	}, func(ok bool) {
		if !ok || nombre.Text == "" || correo.Text == "" {
			return
		}
		cuotaGB, err := strconv.ParseFloat(cuota.Text, 64)
		if err != nil || cuotaGB < 0 {
			cuotaGB = 1
		}
		var servicios []string
		for _, s := range v.catalogos.Servicios {
			servicios = append(servicios, s.Codigo)
		}
		nuevo := bus.NuevoUsuario{
			Nombre:     nombre.Text,
			Correo:     bus.ACorreo(correo.Text),
			Contrasena: clave.Text,
			Rol:        rol.Selected,
			GrupoID:    v.idDeGrupo(grupo.Selected),
			CuotaBytes: int64(cuotaGB * gb),
			Servicios:  servicios,
		}
		enSegundoPlano(func(ctx context.Context) (bus.Usuario, error) {
			return v.app.cli.CrearUsuario(ctx, nuevo)
		}, func(_ bus.Usuario, err error) {
			if err != nil {
				v.app.error(err)
				return
			}
			v.cargar()
		})
	}, v.app.win)
}

func (v *vistaAdministracion) idDeGrupo(nombre string) string {
	for _, g := range v.catalogos.Grupos {
		if g.Nombre == nombre {
			return itoa(g.ID.Int64())
		}
	}
	return ""
}

func (v *vistaAdministracion) acciones(u bus.Usuario) {
	var d dialog.Dialog

	baja := "Dar de baja"
	estadoNuevo := "inactivo"
	if u.Estado != "activo" {
		baja, estadoNuevo = "Reactivar", "activo"
	}

	botonEstado := widget.NewButton(baja, func() {
		d.Hide()
		enSegundoPlano(func(ctx context.Context) (bus.Usuario, error) {
			return v.app.cli.CambiarEstado(ctx, u.ID, estadoNuevo)
		}, v.trasCambiar)
	})

	botonCuota := widget.NewButton("Cambiar la cuota", func() {
		d.Hide()
		entrada := widget.NewEntry()
		entrada.SetText(strconv.FormatFloat(float64(u.CuotaBytes.Int64())/gb, 'f', 2, 64))
		dialog.ShowForm("Cuota de "+u.Nombre, "Guardar", "Cancelar",
			[]*widget.FormItem{widget.NewFormItem("GB", entrada)}, func(ok bool) {
				if !ok {
					return
				}
				gbs, err := strconv.ParseFloat(entrada.Text, 64)
				if err != nil || gbs < 0 {
					return
				}
				enSegundoPlano(func(ctx context.Context) (bus.Usuario, error) {
					return v.app.cli.CambiarCuota(ctx, u.ID, int64(gbs*gb))
				}, v.trasCambiar)
			}, v.app.win)
	})

	detalle := widget.NewLabel(detalleDeCuenta(u))
	d = dialog.NewCustom(u.Nombre, "Cerrar",
		container.NewVBox(detalle, botonEstado, botonCuota), v.app.win)
	d.Show()
}

func (v *vistaAdministracion) trasCambiar(_ bus.Usuario, err error) {
	if err != nil {
		v.app.error(err)
		return
	}
	v.cargar()
}

func (v *vistaAdministracion) sesiones() {
	enSegundoPlano(v.app.cli.ListarSesiones, func(ss []bus.SesionAbierta, err error) {
		if err != nil {
			v.app.error(err)
			return
		}
		if len(ss) == 0 {
			dialog.ShowInformation("Sesiones", "No hay sesiones abiertas.", v.app.win)
			return
		}
		texto := ""
		for _, s := range ss {
			estado := "activa"
			if s.Revocada == "true" {
				estado = "revocada"
			}
			texto += s.Correo + " · " + fecha(s.CreadoEn) + " · " + estado + "\n"
		}
		dialog.ShowInformation("Sesiones abiertas", texto, v.app.win)
	})
}

func resumenDeCuenta(u bus.Usuario) string {
	estado := "activa"
	if u.Estado != "activo" {
		estado = "dada de baja"
	}
	return u.Rol + " · " + estado + " · " + legible(u.CuotaBytes.Int64())
}

func detalleDeCuenta(u bus.Usuario) string {
	t := u.Correo + "\nRol: " + u.Rol + " · estado: " + u.Estado + "\n" +
		"Cuota: " + legible(u.CuotaBytes.Int64()) + " · usa " + legible(u.UsoBytes.Int64()) + "\n"
	if u.Grupo != "" {
		t += "Grupo: " + u.Grupo + "\n"
	}
	if len(u.Servicios) > 0 {
		t += "Servicios: "
		for i, s := range u.Servicios {
			if i > 0 {
				t += ", "
			}
			t += s
		}
	}
	return t
}
