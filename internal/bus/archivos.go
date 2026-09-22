package bus

import (
	"context"
	"io"
	"net/http"
	"path"
	"strconv"
)

// Nodo es un archivo o una carpeta del Home, tal como lo describe el Shared
// File Server. `Ruta` es su identificador: siempre empieza en "/".
type Nodo struct {
	Ruta          string         `json:"ruta"`
	Nombre        string         `json:"nombre"`
	Tipo          string         `json:"tipo"` // imagen, video, dataset, documento…
	EsCarpeta     bool           `json:"esCarpeta"`
	EsEnlace      bool           `json:"esEnlace"`
	Destacado     bool           `json:"destacado"`
	EnPapelera    bool           `json:"enPapelera"`
	TamanoBytes   int64          `json:"tamanoBytes"`
	Version       int64          `json:"version"`
	Propietario   string         `json:"propietario"`
	Grupo         string         `json:"grupo"`
	ModificadoEn  string         `json:"modificadoEn"`
	Permisos      Permisos       `json:"permisos"`
	MiAcceso      string         `json:"miAcceso"`
	CompartidoCon []Comparticion `json:"compartidoCon"`
}

type Permisos struct {
	Octal string `json:"octal"`
	Texto string `json:"texto"`
}

type Comparticion struct {
	Correo  string `json:"correo"`
	Permiso string `json:"permiso"` // lectura | escritura
}

// Listado es lo que devuelve /files para una carpeta.
type Listado struct {
	Carpetas []Nodo `json:"carpetas"`
	Archivos []Nodo `json:"archivos"`
}

// Uso es el resumen de cuota del Home.
type Uso struct {
	Correo     string `json:"correo"`
	UsadoBytes int64  `json:"usadoBytes"`
	CuotaBytes int64  `json:"cuotaBytes"`
	Porcentaje int    `json:"porcentaje"`
	Archivos   int64  `json:"archivos"`
}

// Seccion distingue las vistas del Home que no son una carpeta.
type Seccion string

const (
	MiUnidad   Seccion = ""
	Destacados Seccion = "destacados"
	Papelera   Seccion = "papelera"
)

// Listar devuelve el contenido de una carpeta, o de una sección.
func (c *Cliente) Listar(ctx context.Context, ruta string, seccion Seccion) (Listado, error) {
	params := map[string]string{}
	if seccion != MiUnidad {
		params["seccion"] = string(seccion)
	} else {
		params["ruta"] = oRaiz(ruta)
	}
	var l Listado
	err := c.Pedir(ctx, http.MethodGet, "shared_file", "/files", params, &l)
	return l, err
}

// CompartidosConmigo llega como lista plana, ya con su propietario.
func (c *Cliente) CompartidosConmigo(ctx context.Context) ([]Nodo, error) {
	var ns []Nodo
	err := c.Pedir(ctx, http.MethodGet, "shared_file", "/compartidos-conmigo", nil, &ns)
	return ns, err
}

func (c *Cliente) UsoDelHome(ctx context.Context) (Uso, error) {
	var u Uso
	err := c.Pedir(ctx, http.MethodGet, "shared_file", "/home", nil, &u)
	return u, err
}

func (c *Cliente) CrearCarpeta(ctx context.Context, padre, nombre string) (Nodo, error) {
	var n Nodo
	err := c.Pedir(ctx, http.MethodPost, "shared_file", "/files/carpeta",
		map[string]string{"ruta": oRaiz(padre), "nombre": nombre}, &n)
	return n, err
}

// Subir envía los bytes tal cual; el servicio toma carpeta y nombre de la
// consulta. Si el archivo ya existe, queda como una versión nueva.
func (c *Cliente) Subir(ctx context.Context, padre, nombre string, contenido io.Reader) (Nodo, error) {
	var n Nodo
	err := c.PedirCuerpo(ctx, http.MethodPost, "shared_file", "/files/upload",
		map[string]string{"ruta": oRaiz(padre), "nombre": nombre}, contenido, "", &n)
	return n, err
}

// Descargar entrega el contenido de un archivo. `propietario` solo hace falta
// para lo que otro compartió conmigo.
func (c *Cliente) DescargarArchivo(ctx context.Context, ruta, propietario string) (io.ReadCloser, error) {
	return c.Descargar(ctx, "shared_file", "/files/download",
		map[string]string{"ruta": ruta, "propietario": propietario})
}

func (c *Cliente) Renombrar(ctx context.Context, ruta, nuevoNombre string) (Nodo, error) {
	var n Nodo
	err := c.Pedir(ctx, http.MethodPatch, "shared_file", "/files",
		map[string]string{"ruta": ruta, "nuevoNombre": nuevoNombre}, &n)
	return n, err
}

func (c *Cliente) Destacar(ctx context.Context, ruta string) (Nodo, error) {
	var n Nodo
	err := c.Pedir(ctx, http.MethodPost, "shared_file", "/files/destacar",
		map[string]string{"ruta": ruta}, &n)
	return n, err
}

// Eliminar manda a la papelera; con definitivo, borra de verdad.
func (c *Cliente) Eliminar(ctx context.Context, ruta string, definitivo bool) error {
	params := map[string]string{"ruta": ruta}
	if definitivo {
		params["definitivo"] = "true"
	}
	return c.Pedir(ctx, http.MethodDelete, "shared_file", "/files", params, nil)
}

func (c *Cliente) Restaurar(ctx context.Context, ruta string) (Nodo, error) {
	var n Nodo
	err := c.Pedir(ctx, http.MethodPost, "shared_file", "/files/restaurar",
		map[string]string{"ruta": ruta}, &n)
	return n, err
}

// Version es una entrada del historial de un archivo.
type Version struct {
	Version     int64  `json:"version"`
	TamanoBytes int64  `json:"tamanoBytes"`
	CreadaEn    string `json:"creadaEn"`
	Autor       string `json:"autor"`
}

func (c *Cliente) Versiones(ctx context.Context, ruta string) ([]Version, error) {
	var vs []Version
	err := c.Pedir(ctx, http.MethodGet, "shared_file", "/files/versiones",
		map[string]string{"ruta": ruta}, &vs)
	return vs, err
}

// Compartir da o quita acceso a otra cuenta. permiso: lectura | escritura;
// accion "revoke" retira el acceso.
func (c *Cliente) Compartir(ctx context.Context, ruta, correo, permiso string) error {
	return c.Pedir(ctx, http.MethodPost, "shared_file", "/files/compartir",
		map[string]string{"ruta": ruta, "correo": correo, "permiso": permiso}, nil)
}

func (c *Cliente) DejarDeCompartir(ctx context.Context, ruta, correo string) error {
	return c.Pedir(ctx, http.MethodPost, "shared_file", "/files/compartir",
		map[string]string{"ruta": ruta, "correo": correo, "accion": "revoke"}, nil)
}

// CambiarPermisos fija los permisos Unix del nodo (por ejemplo "640").
func (c *Cliente) CambiarPermisos(ctx context.Context, ruta, octal string) (Nodo, error) {
	var n Nodo
	err := c.Pedir(ctx, http.MethodPut, "shared_file", "/files/permisos",
		map[string]string{"ruta": ruta, "modo": octal}, &n)
	return n, err
}

// Unir junta una carpeta con un nombre: "/" + "a" = "/a", "/x" + "a" = "/x/a".
func Unir(padre, nombre string) string {
	if padre == "" || padre == "/" {
		return "/" + nombre
	}
	return path.Join(padre, nombre)
}

// Padre devuelve la carpeta que contiene a una ruta.
func Padre(ruta string) string {
	p := path.Dir(ruta)
	if p == "." {
		return "/"
	}
	return p
}

func oRaiz(ruta string) string {
	if ruta == "" {
		return "/"
	}
	return ruta
}

// Legible pasa unos bytes a algo que se pueda leer de un vistazo.
func Legible(bytes int64) string {
	const unidad = 1024
	if bytes < unidad {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	div, exp := int64(unidad), 0
	for n := bytes / unidad; n >= unidad && exp < 3; n /= unidad {
		div *= unidad
		exp++
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 1, 64) + " " + [...]string{"KB", "MB", "GB", "TB"}[exp]
}
