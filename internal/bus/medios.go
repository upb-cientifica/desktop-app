package bus

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

// ---------- Álbum de fotos ----------

type Album struct {
	ID          string `json:"id"`
	Titulo      string `json:"titulo"`
	Descripcion string `json:"descripcion"`
	Proyecto    string `json:"proyecto"`
	NumImagenes Numero `json:"numImagenes"`
	MiRol       string `json:"miRol"`
	Propietario string `json:"propietario"`
}

type Imagen struct {
	ID          string `json:"id"`
	AlbumID     string `json:"albumId"`
	Titulo      string `json:"titulo"`
	Descripcion string `json:"descripcion"`
	Etiquetas   Lista  `json:"etiquetas"`
	Ancho       Numero `json:"ancho"`
	Alto        Numero `json:"alto"`
	TamanoBytes Numero `json:"tamanoBytes"`
	SubidaEn    string `json:"subidaEn"`
	OrigenHome  string `json:"origenHome"`
}

func (c *Cliente) Albums(ctx context.Context) ([]Album, error) {
	var as []Album
	err := c.Pedir(ctx, http.MethodGet, "photo_album", "/albums", nil, &as)
	return as, err
}

// Fotos devuelve todas las imágenes visibles, o las de un álbum.
func (c *Cliente) Fotos(ctx context.Context, albumID string) ([]Imagen, error) {
	var ims []Imagen
	ruta := "/buscar"
	if albumID != "" {
		ruta = "/albums/" + url.PathEscape(albumID) + "/imagenes"
	}
	err := c.Pedir(ctx, http.MethodGet, "photo_album", ruta, nil, &ims)
	return ims, err
}

// Miniatura y Imagen entregan los bytes de una foto.
func (c *Cliente) Miniatura(ctx context.Context, id string) (io.ReadCloser, error) {
	return c.Descargar(ctx, "photo_album", "/imagenes/"+url.PathEscape(id)+"/miniatura", nil)
}

func (c *Cliente) ImagenCompleta(ctx context.Context, id string) (io.ReadCloser, error) {
	return c.Descargar(ctx, "photo_album", "/imagenes/"+url.PathEscape(id), nil)
}

// CrearAlbum y AgregarImagenDelHome publican en Fotos una imagen que ya está
// en el Home: el servicio la recoge por RMI, no se vuelve a subir.
func (c *Cliente) CrearAlbum(ctx context.Context, titulo string) (Album, error) {
	var a Album
	err := c.Pedir(ctx, http.MethodPost, "photo_album", "/albums",
		map[string]string{"titulo": titulo}, &a)
	return a, err
}

func (c *Cliente) AgregarImagenDelHome(ctx context.Context, albumID, rutaHome, titulo string) (Imagen, error) {
	var im Imagen
	err := c.Pedir(ctx, http.MethodPost, "photo_album",
		"/albums/"+url.PathEscape(albumID)+"/imagenes",
		map[string]string{"homeRuta": rutaHome, "titulo": titulo}, &im)
	return im, err
}

// EliminarImagen quita una foto del álbum.
func (c *Cliente) EliminarImagen(ctx context.Context, id string) error {
	return c.Pedir(ctx, http.MethodDelete, "photo_album", "/imagenes/"+url.PathEscape(id), nil, nil)
}

// ---------- Streaming ----------

type Video struct {
	ID          string `json:"id"`
	Titulo      string `json:"titulo"`
	Autor       string `json:"autor"`
	Proyecto    string `json:"proyecto"`
	NivelAcceso string `json:"nivelAcceso"`
	DuracionSeg Numero `json:"duracionSeg"`
	TamanoBytes Numero `json:"tamanoBytes"`
	PublicadoEn string `json:"publicadoEn"`
	HlsListo    bool   `json:"hlsListo"`
	OrigenHome  string `json:"origenHome"` // archivo del Home del que salió
}

func (c *Cliente) Videos(ctx context.Context) ([]Video, error) {
	var vs []Video
	err := c.Pedir(ctx, http.MethodGet, "streaming", "/videos",
		map[string]string{"orden": "fecha"}, &vs)
	return vs, err
}

func (c *Cliente) Video(ctx context.Context, id string) (Video, error) {
	var v Video
	err := c.Pedir(ctx, http.MethodGet, "streaming", "/videos/"+url.PathEscape(id), nil, &v)
	return v, err
}

// URLDelManifiesto es la dirección HLS del video, con el token en la consulta.
//
// El reproductor del sistema no manda encabezados, así que la identidad viaja
// en la URL: tanto el bus como el servicio aceptan `?token=` además del
// encabezado, justo para estos casos.
func (c *Cliente) URLDelManifiesto(id string) string {
	return c.URL("streaming", "/videos/"+url.PathEscape(id)+"/index.m3u8",
		map[string]string{"token": c.Token()})
}

// EliminarVideo quita un video del catálogo de Streaming.
func (c *Cliente) EliminarVideo(ctx context.Context, id string) error {
	return c.Pedir(ctx, http.MethodDelete, "streaming", "/videos/"+url.PathEscape(id), nil, nil)
}

// ImportarVideoDelHome publica en Streaming un video que ya está en el Home.
// El servicio lo trae por RMI y lo empaqueta en HLS con ffmpeg.
func (c *Cliente) ImportarVideoDelHome(ctx context.Context, rutaHome, titulo string) (Video, error) {
	var v Video
	err := c.Pedir(ctx, http.MethodPost, "streaming", "/videos/importar",
		map[string]string{"ruta": rutaHome, "titulo": titulo}, &v)
	return v, err
}

// ---------- Trabajos del clúster (Java RMI mediado por el bus) ----------

type Trabajo struct {
	ID           string `json:"id"`
	Nombre       string `json:"nombre"`
	Estado       string `json:"estado"` // ENCOLADO, EJECUTANDO, COMPLETADO, FALLIDO, CANCELADO
	Procesos     Numero `json:"procesos"`
	Progreso     Numero `json:"progreso"`
	CodigoSalida Numero `json:"codigoSalida"`
	Mensaje      string `json:"mensaje"`
	DuracionSeg  Numero `json:"duracionSeg"`
	CreadoEn     string `json:"creadoEn"`
	Propietario  string `json:"propietario"`
	Comando      string `json:"comando"`
	RutaHome     string `json:"rutaHome"`
}

// NodoCluster es una máquina del clúster HPC (no confundir con Nodo, que es
// un archivo o carpeta del Home).
type NodoCluster struct {
	Host       string `json:"host"`
	Slots      Numero `json:"slots"`
	Disponible bool   `json:"disponible"`
}

func (c *Cliente) Trabajos(ctx context.Context) ([]Trabajo, error) {
	var ts []Trabajo
	err := c.Pedir(ctx, http.MethodGet, "hpc", "/trabajos", nil, &ts)
	return ts, err
}

func (c *Cliente) NodosDelCluster(ctx context.Context) ([]NodoCluster, error) {
	var ns []NodoCluster
	err := c.Pedir(ctx, http.MethodGet, "hpc", "/nodos", nil, &ns)
	return ns, err
}

func (c *Cliente) EnviarTrabajo(ctx context.Context, nombre, comando, rutaHome string, procesos int) (Trabajo, error) {
	var t Trabajo
	err := c.Pedir(ctx, http.MethodPost, "hpc", "/trabajos", map[string]string{
		"nombre": nombre, "comando": comando, "rutaHome": rutaHome, "procesos": itoa(int64(procesos)),
	}, &t)
	return t, err
}

func (c *Cliente) CancelarTrabajo(ctx context.Context, id string) error {
	return c.Pedir(ctx, http.MethodDelete, "hpc", "/trabajos/"+url.PathEscape(id), nil, nil)
}

func (c *Cliente) SalidaDelTrabajo(ctx context.Context, id string) (string, error) {
	var r struct {
		Salida string `json:"salida"`
	}
	err := c.Pedir(ctx, http.MethodGet, "hpc", "/trabajos/"+url.PathEscape(id)+"/salida", nil, &r)
	return r.Salida, err
}

// ---------- Monitoreo ----------

type Host struct {
	CPUPct     float64 `json:"cpuPct"`
	MemoriaPct float64 `json:"memoriaPct"`
	DiscoPct   float64 `json:"discoPct"`
	Uptime     string  `json:"uptime"`
}

type ServicioVigilado struct {
	Nombre     string `json:"nombre"`
	Estado     string `json:"estado"` // disponible | caido
	LatenciaMs Numero `json:"latenciaMs"`
	RevisadoEn string `json:"revisadoEn"`
}

func (c *Cliente) HostMonitoreado(ctx context.Context) (Host, error) {
	var h Host
	err := c.Pedir(ctx, http.MethodGet, "monitoreo", "/host", nil, &h)
	return h, err
}

func (c *Cliente) ServiciosVigilados(ctx context.Context) ([]ServicioVigilado, error) {
	var ss []ServicioVigilado
	err := c.Pedir(ctx, http.MethodGet, "monitoreo", "/servicios", nil, &ss)
	return ss, err
}

type Alerta struct {
	ID      string `json:"id"`
	Metrica string `json:"metrica"`
	Umbral  Numero `json:"umbral"`
	Activa  bool   `json:"activa"`
}

func (c *Cliente) Alertas(ctx context.Context) ([]Alerta, error) {
	var r struct {
		Reglas []Alerta `json:"reglas"`
	}
	err := c.Pedir(ctx, http.MethodGet, "monitoreo", "/alertas", nil, &r)
	return r.Reglas, err
}
