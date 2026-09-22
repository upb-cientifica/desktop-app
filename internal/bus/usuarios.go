package bus

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// DominioCorreo se añade cuando el usuario escribe solo su cuenta.
const DominioCorreo = "upb.edu.co"

// Usuario es lo que el directorio sabe de una cuenta.
//
// El directorio es SOAP y el bus traduce el XML a JSON. En ese XML un
// elemento que aparece una sola vez no se distingue de una lista de uno, así
// que `servicios` llega unas veces como cadena y otras como arreglo: por eso
// es un Lista y no un []string.
type Usuario struct {
	ID         string `json:"id"`
	Correo     string `json:"correo"`
	Nombre     string `json:"nombre"`
	Rol        string `json:"rol"`
	Estado     string `json:"estado"`
	Grupo      string `json:"grupo"`
	CuotaBytes Numero `json:"cuotaBytes"`
	UsoBytes   Numero `json:"usoBytes"`
	Servicios  Lista  `json:"servicios"`
	CreadoEn   string `json:"creadoEn"`
}

func (u Usuario) EsAdmin() bool { return u.Rol == "admin" }

// Sesion es lo que devuelve un inicio de sesión correcto.
type Sesion struct {
	AccessToken  string  `json:"accessToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiraEn     Numero  `json:"expiraEn"`
	Usuario      Usuario `json:"usuario"`
}

// Login pide un par de tokens al directorio. El correo puede venir completo o
// solo con la cuenta.
func (c *Cliente) Login(ctx context.Context, cuenta, password string) (Sesion, error) {
	var s Sesion
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/login", map[string]string{
		"correo":   ACorreo(cuenta),
		"password": password,
	}, &s)
	return s, err
}

// ACorreo completa el dominio institucional si hace falta.
func ACorreo(cuenta string) string {
	cuenta = strings.TrimSpace(cuenta)
	if cuenta == "" || strings.Contains(cuenta, "@") {
		return cuenta
	}
	return cuenta + "@" + DominioCorreo
}

// Renovar canjea el token de refresco por uno de acceso nuevo. El servidor
// rota el de refresco en cada uso y devuelve el siguiente.
func (c *Cliente) Renovar(ctx context.Context, refresco string) (Sesion, error) {
	var s Sesion
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/renovarToken",
		map[string]string{"refreshToken": refresco}, &s)
	return s, err
}

// MiPerfil devuelve la cuenta del token actual.
func (c *Cliente) MiPerfil(ctx context.Context) (Usuario, error) {
	var r struct {
		Usuario Usuario `json:"usuario"`
	}
	if err := c.Pedir(ctx, http.MethodPost, "usuarios", "/miPerfil", nil, &r); err != nil {
		return Usuario{}, err
	}
	if r.Usuario.Correo != "" {
		return r.Usuario, nil
	}
	// Según la operación, el bus devuelve el usuario ya desenvuelto.
	var u Usuario
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/miPerfil", nil, &u)
	return u, err
}

// CerrarSesion revoca la sesión en el servidor.
func (c *Cliente) CerrarSesion(ctx context.Context) error {
	return c.Pedir(ctx, http.MethodPost, "usuarios", "/cerrarSesion", nil, nil)
}

// ---------- Administración ----------

type Pagina[T any] struct {
	Items ListaDe[T] `json:"items"`
	Total Numero     `json:"total"`
}

// ListarUsuarios exige rol admin en el directorio.
func (c *Cliente) ListarUsuarios(ctx context.Context) ([]Usuario, error) {
	var p Pagina[Usuario]
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/listarUsuarios",
		map[string]string{"tamano": "200"}, &p)
	return p.Items, err
}

type Catalogos struct {
	Roles     ListaDe[Catalogo] `json:"roles"`
	Grupos    ListaDe[Grupo]    `json:"grupos"`
	Servicios ListaDe[Catalogo] `json:"servicios"`
}

type Catalogo struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}

type Grupo struct {
	ID     Numero `json:"id"`
	Nombre string `json:"nombre"`
}

func (c *Cliente) Catalogos(ctx context.Context) (Catalogos, error) {
	var cat Catalogos
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/listarCatalogos", nil, &cat)
	return cat, err
}

// NuevoUsuario son los campos con los que se da de alta una cuenta.
type NuevoUsuario struct {
	Nombre     string
	Correo     string
	Contrasena string
	Rol        string
	GrupoID    string
	CuotaBytes int64
	Servicios  []string
}

// CrearUsuario da de alta una cuenta.
//
// El WSDL agrupa los campos dentro de <datos>; el bus arma ese anidamiento a
// partir del prefijo "datos." del parámetro, y separa por comas lo que el
// esquema permite repetir (los servicios).
func (c *Cliente) CrearUsuario(ctx context.Context, n NuevoUsuario) (Usuario, error) {
	return c.usuarioDe(ctx, "/crearUsuario", map[string]string{
		"datos.correo":     n.Correo,
		"datos.nombre":     n.Nombre,
		"datos.contrasena": n.Contrasena,
		"datos.rol":        n.Rol,
		"datos.grupoId":    n.GrupoID,
		"datos.cuotaBytes": itoa(n.CuotaBytes),
		"datos.servicios":  strings.Join(n.Servicios, ","),
	})
}

// CambiarEstado activa o da de baja una cuenta ("activo" / "inactivo").
func (c *Cliente) CambiarEstado(ctx context.Context, id, estado string) (Usuario, error) {
	return c.usuarioDe(ctx, "/actualizarUsuario", map[string]string{
		"id": id, "datos.estado": estado,
	})
}

// CambiarCuota fija la cuota de una cuenta, en bytes.
func (c *Cliente) CambiarCuota(ctx context.Context, id string, bytes int64) (Usuario, error) {
	return c.usuarioDe(ctx, "/actualizarUsuario", map[string]string{
		"id": id, "datos.cuotaBytes": itoa(bytes),
	})
}

func (c *Cliente) usuarioDe(ctx context.Context, op string, params map[string]string) (Usuario, error) {
	var crudo json.RawMessage
	if err := c.Pedir(ctx, http.MethodPost, "usuarios", op, params, &crudo); err != nil {
		return Usuario{}, err
	}
	var envuelto struct {
		Usuario Usuario `json:"usuario"`
	}
	if json.Unmarshal(crudo, &envuelto) == nil && envuelto.Usuario.Correo != "" {
		return envuelto.Usuario, nil
	}
	var u Usuario
	err := json.Unmarshal(crudo, &u)
	return u, err
}

// Sesiones es la bitácora que lleva el directorio: quién tiene sesión abierta.
type SesionAbierta struct {
	ID          string `json:"id"`
	Correo      string `json:"correo"`
	Dispositivo string `json:"dispositivo"`
	IP          string `json:"ip"`
	CreadoEn    string `json:"creadoEn"`
	Revocada    string `json:"revocada"`
}

func (c *Cliente) ListarSesiones(ctx context.Context) ([]SesionAbierta, error) {
	var p Pagina[SesionAbierta]
	err := c.Pedir(ctx, http.MethodPost, "usuarios", "/listarSesiones", nil, &p)
	return p.Items, err
}
